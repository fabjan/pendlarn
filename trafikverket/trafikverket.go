package trafikverket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"time"
)

type LightAnnouncement struct {
	AdvertisedTrainIdent string `json:"AdvertisedTrainIdent"`
}

type TrainAnnouncement struct {
	LocationSignature        string        `json:"LocationSignature"`
	AdvertisedTimeAtLocation string        `json:"AdvertisedTimeAtLocation"`
	AdvertisedTrainIdent     string        `json:"AdvertisedTrainIdent"`
	Operator                 string        `json:"Operator"`
	OtherInformation         []Information `json:"OtherInformation"`
	Deviation                []Information `json:"Deviation"`
	TrackAtLocation          string        `json:"TrackAtLocation"`
}

func (t *TrainAnnouncement) ParseTime() (time.Time, error) {
	return time.Parse("2006-01-02T15:04:05", t.AdvertisedTimeAtLocation)
}

type Information struct {
	Code        string `json:"Code"`
	Description string `json:"Description"`
}

type listTrainsParams struct {
	ApiKey      string
	After       string
	Before      string
	FromStation string
	ToStation   string
}

// find all trains that stop at either from or to within the given time frame
var listTrainsQuery = template.Must(template.New("listTrainsQuery").Parse(`
<REQUEST>
	<LOGIN authenticationkey="{{ .ApiKey }}"/>
	<QUERY objecttype="TrainAnnouncement" orderby="AdvertisedTimeAtLocation" schemaversion="1.8">
		<INCLUDE>AdvertisedTrainIdent</INCLUDE>
		<FILTER>
			<AND>
				<GT name="AdvertisedTimeAtLocation" value="{{ .After }}"/>
				<LT name="AdvertisedTimeAtLocation" value="{{ .Before }}"/>
				<OR>
					<AND>
                        <EQ name="ActivityType" value="Avgang"/>
                        <EQ name="LocationSignature" value="{{ .FromStation }}"/>
						<OR>
                        	<EQ name="ToLocation.LocationName" value="{{ .ToStation }}"/>
							<EQ name="ViaToLocation.LocationName" value="{{ .ToStation }}"/>
						</OR>
                    </AND>
                    <AND>
                        <EQ name="ActivityType" value="Ankomst"/>
                        <EQ name="LocationSignature" value="{{ .ToStation }}"/>
						<OR>
							<EQ name="FromLocation.LocationName" value="{{ .FromStation }}"/>
							<EQ name="ViaFromLocation.LocationName" value="{{ .FromStation }}"/>
						</OR>
                    </AND>
				</OR>
			</AND>
		</FILTER>
	</QUERY>
</REQUEST>
`))

type getAnnouncementParams struct {
	ApiKey   string
	TrainIds []string
	After    string
	Before   string
	From     string
}

// get all announcements for the given train IDs in the given time frame departing from the given station
var getAnnouncementsQuery = template.Must(template.New("getAnnouncementQuery").Parse(`
<REQUEST>
	<LOGIN authenticationkey="{{ .ApiKey }}"/>
	<QUERY objecttype="TrainAnnouncement" orderby="AdvertisedTimeAtLocation" schemaversion="1.8">
		<INCLUDE>LocationSignature</INCLUDE>
		<INCLUDE>AdvertisedTimeAtLocation</INCLUDE>
		<INCLUDE>AdvertisedTrainIdent</INCLUDE>
		<INCLUDE>Operator</INCLUDE>
		<INCLUDE>Deviation</INCLUDE>
		<INCLUDE>OtherInformation</INCLUDE>
		<INCLUDE>TrackAtLocation</INCLUDE>
		<FILTER>
			<AND>
				<EQ name="ActivityType" value="Avgang"/>
				<EQ name="LocationSignature" value="{{ .From }}"/>
				<GT name="AdvertisedTimeAtLocation" value="{{ .After }}"/>
				<LT name="AdvertisedTimeAtLocation" value="{{ .Before }}"/>
				<IN name="AdvertisedTrainIdent" value="{{ range .TrainIds }}{{ . }},{{ end }}"/>
			</AND>
		</FILTER>
	</QUERY>
</REQUEST>
`))

// GetTrainsStoppingAtEither returns a list of train IDs that stop at from and to within the given time frame.
func GetTrainsStoppingAt(apiKey string, from string, to string, after time.Time, before time.Time) (error, []TrainAnnouncement) {

	if !after.Before(before) {
		return fmt.Errorf("after must be before before"), nil
	}

	listQuery := listTrainsParams{
		ApiKey:      apiKey,
		After:       toTimestamp(after),
		Before:      toTimestamp(before),
		FromStation: from,
		ToStation:   to,
	}
	buf := new(bytes.Buffer)
	err := listTrainsQuery.Execute(buf, listQuery)
	if err != nil {
		return err, nil
	}

	resp, err := postToAPI(buf)
	if err != nil {
		return err, nil
	}

	var listData struct {
		RESPONSE struct {
			RESULT []struct {
				TrainIDs []LightAnnouncement `json:"TrainAnnouncement"`
			} `json:"RESULT"`
		} `json:"RESPONSE"`
	}
	err = json.NewDecoder(resp.Body).Decode(&listData)
	if err != nil {
		return err, nil
	}

	seenIDs := map[string]bool{}
	trainIDs := []string{}
	for _, result := range listData.RESPONSE.RESULT {
		for _, announcement := range result.TrainIDs {
			if !seenIDs[announcement.AdvertisedTrainIdent] {
				seenIDs[announcement.AdvertisedTrainIdent] = true
				trainIDs = append(trainIDs, announcement.AdvertisedTrainIdent)
			}
		}
	}

	getQuery := getAnnouncementParams{
		ApiKey:   apiKey,
		TrainIds: trainIDs,
		From:     from,
		After:    toTimestamp(after),
		Before:   toTimestamp(before),
	}
	buf = new(bytes.Buffer)
	err = getAnnouncementsQuery.Execute(buf, getQuery)
	if err != nil {
		return err, nil
	}

	resp, err = postToAPI(buf)
	if err != nil {
		return err, nil
	}

	var announceData struct {
		RESPONSE struct {
			RESULT []struct {
				TrainAnnouncement []TrainAnnouncement `json:"TrainAnnouncement"`
			} `json:"RESULT"`
		} `json:"RESPONSE"`
	}
	err = json.NewDecoder(resp.Body).Decode(&announceData)
	if err != nil {
		return err, nil
	}

	announcements := []TrainAnnouncement{}
	for _, result := range announceData.RESPONSE.RESULT {
		for _, announcement := range result.TrainAnnouncement {
			announcements = append(announcements, announcement)
		}
	}

	return nil, announcements
}

func postToAPI(body io.Reader) (*http.Response, error) {
	url := "https://api.trafikinfo.trafikverket.se/v2/data.json"

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml")

	resp, err := http.DefaultClient.Do(req)
	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("unauthorized (check your API key)")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return resp, nil
}

func toTimestamp(t time.Time) string {
	// a time in ISO 8601 format, with zone offset. e.g. 2019-01-01T12:00:00+01:00
	return t.Format("2006-01-02T15:04:05-07:00")
}
