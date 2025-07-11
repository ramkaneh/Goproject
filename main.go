package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Структуры для данных
type Airport struct {
	Code string
	Name string
}

type Flight struct {
	ID     int
	Number string
}

type Aircraft struct {
	Code string
}

type Result struct {
	AircraftCode string
	Square       int
	TimeTaken    time.Duration
}

type FlightsPageData struct {
	AirportCode string
	Flights     []Flight
}

type CalculationPageData struct {
	Results []Result
}

var db *pgxpool.Pool
var tmpl *template.Template

func main() {
	// Инициализация шаблонов
	initTemplates()

	// Подключение к БД
	connStr := "user=postgres dbname=demo host=********* port=5432 sslmode=disable" //"postgres://postgres:@*********:5432/demo"
	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer pool.Close()
	db = pool

	// Роутинг
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/airports", airportsHandler)
	http.HandleFunc("/flights", flightsHandler)
	http.HandleFunc("/aircrafts", aircraftsHandler)
	http.HandleFunc("/aircrafts/calculate", calculateHandler)

	// Запуск сервера
	log.Println("Server started on :8080")
	http.ListenAndServe(":8080", nil)
}

func initTemplates() {
	tmpl = template.Must(template.ParseGlob("templates/*.html"))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/airports", http.StatusFound)
}

func airportsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(context.Background(), "SELECT airport_code, airport_name FROM bookings.airports")
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var airports []Airport
	for rows.Next() {
		var a Airport
		if err := rows.Scan(&a.Code, &a.Name); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		airports = append(airports, a)
	}

	tmpl.ExecuteTemplate(w, "airports.html", airports)
}

func flightsHandler(w http.ResponseWriter, r *http.Request) {
	airportCode := r.URL.Query().Get("airport")
	if airportCode == "" {
		http.Error(w, "Airport code required", http.StatusBadRequest)
		return
	}

	rows, err := db.Query(context.Background(), `
		SELECT flight_id, flight_no 
		FROM bookings.flights 
		WHERE departure_airport = $1
	`, airportCode)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var flights []Flight
	for rows.Next() {
		var f Flight
		if err := rows.Scan(&f.ID, &f.Number); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		flights = append(flights, f)
	}

	data := FlightsPageData{
		AirportCode: airportCode,
		Flights:     flights,
	}

	tmpl.ExecuteTemplate(w, "flights.html", data)
}

func aircraftsHandler(w http.ResponseWriter, r *http.Request) {
	tmpl.ExecuteTemplate(w, "aircrafts.html", nil)
}

func calculateHandler(w http.ResponseWriter, r *http.Request) {
	// Получаем список самолетов
	rows, err := db.Query(context.Background(), "SELECT aircraft_code FROM bookings.aircrafts_data")
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var aircrafts []Aircraft
	for rows.Next() {
		var a Aircraft
		if err := rows.Scan(&a.Code); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		aircrafts = append(aircrafts, a)
	}

	// WaitGroup для ожидания завершения горутин
	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []Result

	for _, aircraft := range aircrafts {
		wg.Add(1)
		go func(a Aircraft) {
			defer wg.Done()

			start := time.Now()
			sum := calculateSeatsSum(a.Code)
			square := sum * sum
			timeTaken := time.Since(start)

			mu.Lock()
			results = append(results, Result{
				AircraftCode: a.Code,
				Square:       square,
				TimeTaken:    timeTaken,
			})
			mu.Unlock()
		}(aircraft)
	}

	wg.Wait()

	data := CalculationPageData{
		Results: results,
	}

	tmpl.ExecuteTemplate(w, "results.html", data)
}

func calculateSeatsSum(aircraftCode string) int {
	rows, err := db.Query(context.Background(), `
		SELECT seat_no 
		FROM bookings.seats 
		WHERE aircraft_code = $1
	`, aircraftCode)
	if err != nil {
		log.Printf("Seats query error: %v", err)
		return 0
	}
	defer rows.Close()

	sum := 0
	for rows.Next() {
		var seat string
		if err := rows.Scan(&seat); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		if num, err := strconv.Atoi(seat[:len(seat)-1]); err == nil {
			sum += num
		}
	}
	return sum
}
