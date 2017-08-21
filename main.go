package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// APIKeys is a structure...
type APIKeys struct {
	Openweathermap     string `json:"openweathermap"`
	Weatherunderground string `json:"weatherunderground"`
}

func main() {
	var (
		flAddr   = flag.String("http.addr", ":8080", "http server address")
		flConfig = flag.String("config", "config.json", "path to configuration file")
	)
	flag.Parse()

	file, err := ioutil.ReadFile(*flConfig)
	if err != nil {
		fmt.Printf("File error: %v\n", err)
		os.Exit(1)
	}

	var jsontype APIKeys
	json.Unmarshal(file, &jsontype)
	fmt.Printf("Results: %v\n", jsontype)

	mw := multyWeatherProvider{
		openWeatherMap{apiKey: jsontype.Openweathermap},
		weatherUnderground{apiKey: jsontype.Weatherunderground},
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			index(w, r)
			return
		case "/hello":
			hello(w, r)
			return
		default:
			http.Error(w, "not found", 404)
		}
	})

	http.HandleFunc("/form", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
		<html>
		<form action="/weather/Chisinau" method="get" id="form1">
	  	</form>
	  
		  <button type="submit" form="form1" value="Submit">Submit</button>
		  </html>
		`))

	})

	http.HandleFunc("/weather/", func(w http.ResponseWriter, r *http.Request) {
		begin := time.Now()
		city := strings.SplitN(r.URL.Path, "/", 3)[2]

		temp, err := mw.temperature(city)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"city": city,
			"temp": temp,
			"took": time.Since(begin).String(),
		})
	})

	http.ListenAndServe(*flAddr, nil)

	http.HandleFunc("/hello", hello)

}

func index(w http.ResponseWriter, r *http.Request) {

	w.Write([]byte(`
	<html>
	<form action="/weather/Chisinau" method="get" id="form1">
	  </form>
  
	  <button type="submit" form="form1" value="Submit">Submit</button>
	  </html>
	`))

}

func hello(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello"))
}

type weatherData struct {
	Name string `json:"name"`
	Main struct {
		Kelvin float64 `json:"temp"`
	} `json:"main"`
}

type weatherProvider interface {
	temperature(city string) (float64, error)
}

type openWeatherMap struct {
	apiKey string
}

func (w openWeatherMap) temperature(city string) (float64, error) {
	resp, err := http.Get("http://api.openweathermap.org/data/2.5/weather?appid=" + w.apiKey + "q=" + city)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var d weatherData
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	d.Main.Kelvin = d.Main.Kelvin - 273.15
	log.Printf("openWeatherMap: %s: %.2f", city, d.Main.Kelvin)

	return d.Main.Kelvin, nil
}

type weatherUnderground struct {
	apiKey string
}

func (w weatherUnderground) temperature(city string) (float64, error) {
	time.Sleep(10 * time.Second)
	resp, err := http.Get("http://api.wunderground.com/api/" + w.apiKey + "/conditions/q/" + city + ".json")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var d struct {
		Observation struct {
			Celsius float64 `json:"temp_c"`
		} `json:"current_observation"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, nil
	}
	log.Printf("weatherUnderground: %s: %.2f", city, d.Observation.Celsius)
	return d.Observation.Celsius, nil
}

type multyWeatherProvider []weatherProvider

func (w multyWeatherProvider) temperature(city string) (float64, error) {
	temps := make(chan float64, len(w))
	errs := make(chan error, len(w))
	var sum float64

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	request := func(p weatherProvider) {
		temp, err := p.temperature(city)
		if err != nil {
			errs <- err
			return
		}
		temps <- temp
	}

	for _, provider := range w {
		go request(provider)
	}

	var p float64

	for range w {
		select {
		case temp := <-temps:
			sum += temp
			p++
		case err := <-errs:
			return 0, err
		case <-ctx.Done():
			fmt.Println("ctx is done")

		}
	}
	return sum / p, nil

}
