package main

import (
	_ "embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/fabjan/pendlarn/trafikverket"
)

var TRAFIKVERKET_API_KEY = os.Getenv("TRAFIKVERKET_API_KEY")
var PORT = os.Getenv("PORT")

//go:embed static/style.css
var style string

//go:embed static/favicon.ico
var favicon []byte

//go:embed static/icon-512.png
var icon512 []byte

func main() {
	if TRAFIKVERKET_API_KEY == "" {
		log.Fatal("TRAFIKVERKET_API_KEY environment variable not set")
	}

	if PORT == "" {
		PORT = "3000"
	}

	// who needs a web framework or server anyway
	http.HandleFunc("/static/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		appManifest := `{
			"name": "Pendlarn",
			"short_name": "Pendlarn",
			"icons": [
				{
					"src": "/static/icon-512.png",
					"sizes": "512x512",
					"type": "image/png",
					"purpose": "any maskable"
				}
			],
			"theme_color": "#007bff",
			"background_color": "smokewhite",
			"display": "standalone"
		}`

		fmt.Fprintf(w, appManifest)
	})
	http.HandleFunc("/static/icon-512.png", func(w http.ResponseWriter, r *http.Request) {
		w.Write(icon512)
	})
	http.HandleFunc("/static/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Write(favicon)
	})

	http.HandleFunc("/", http.RedirectHandler("/now/uppsala", http.StatusFound).ServeHTTP)
	http.HandleFunc("/now/uppsala", nowHandler("U", "Cst"))
	http.HandleFunc("/now/stockholm", nowHandler("Cst", "U"))

	fmt.Println("Listening on port " + PORT)
	http.ListenAndServe(":"+PORT, nil)
}

func nowHandler(from string, to string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		serveSearch(w, r, from, to, time.Now())
	}
}

func serveSearch(w http.ResponseWriter, r *http.Request, from string, to string, after time.Time) {

	// look for trains departing in a 1 hour window
	window := 1 * time.Hour
	// account for travel time (we are looking for trains that are stopping at the destination)
	duration := 2 * window

	before := after.Add(duration)

	err, trains := trafikverket.GetTrainsStoppingAt(TRAFIKVERKET_API_KEY, from, to, after, before)
	if err != nil {
		fmt.Println("Error getting trains: ", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	renderResults(w, r, from, to, trains)
}

func navBar(currentLocation string) string {
	var items []struct{ key, link string } = []struct{ key, link string }{
		{"U", "uppsala"},
		{"Cst", "stockholm"},
	}

	nav := "<nav><ul>"
	for _, item := range items {
		name := locationName(item.key)
		if item.key == currentLocation {
			nav += "<li class=\"here\"><a href=\"javascript:;\">" + name + "</a></li>"
		} else {
			nav += "<li><a href=\"/now/" + item.link + "\">" + name + "</a></li>"
		}
	}
	nav += "</ul></nav>"

	return nav
}

func renderResults(w http.ResponseWriter, r *http.Request, from string, to string, trains []trafikverket.TrainAnnouncement) {

	title := locationName(from) + " &rarr;"

	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
	<title> Pendlarn - `+title+`</title>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<meta name="theme-color" content="#007bff">
	<link rel="icon" href="/static/favicon.ico" type="image/x-icon">
	<link rel="apple-touch-icon" href="/static/icon-512.png">
	<link rel="manifest" href="/static/manifest.json">
	<link rel="stylesheet" href="https://unpkg.com/sakura.css@1.4.1/css/sakura.css" type="text/css">
	<style>`+style+`</style>
</head>
<body>
	<header>
		`+navBar(from)+`
	</header>
	<main>
	`)

	fmt.Fprintf(w, `
		<table>
			<tbody>
	`)

	for _, train := range trains {
		fmt.Fprint(w, announcementToRow(train))
	}

	fmt.Fprintf(w, `
			</tbody>
		</table>
	`)
}

func announcementToRow(ann trafikverket.TrainAnnouncement) string {

	// slice out the hours and minutes
	atTime := ann.AdvertisedTimeAtLocation[11:16]
	parsed, err := ann.ParseTime()
	if err == nil {
		atTime = parsed.Format("15:04")
	}

	row := "<tr>"
	row += "<td>" + atTime + "</td>"
	row += "<td>" + ann.TrackAtLocation + "</td>"
	row += "<td>" + ann.Operator + "</td>"
	row += "<td>" + ann.AdvertisedTrainIdent + "</td>"
	row += "<td>"
	for _, info := range ann.Deviation {
		row += "<span class=\"deviation\">" + info.Description + "</span><br>"
	}
	for _, info := range ann.OtherInformation {
		if uninteresting(info) {
			continue
		}
		row += "<span class=\"info\">" + info.Description + "</span><br>"
	}
	row += "</td>"
	row += "</tr>"
	return row
}

func uninteresting(info trafikverket.Information) bool {

	switch info.Code {
	case "ONA151":
		// stannar ej i Märsta
		return true
	case "ONA124":
		// buss ersätter
		return true
	case "ONA001":
		// ej avstigning från X vagnar
		return true
	}

	return false
}

func locationName(locationSignature string) string {

	switch locationSignature {
	case "U":
		return "Uppsala"
	case "Cst":
		return "Stockholm C"
	}

	return locationSignature
}
