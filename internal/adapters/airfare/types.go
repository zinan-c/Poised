package airfare

import "time"

type PriceObservation struct {
	Adapter         string    `json:"adapter"`
	RouteName       string    `json:"route_name,omitempty"`
	Airline         string    `json:"airline,omitempty"`
	FlightNo        string    `json:"flight_no,omitempty"`
	Origin          string    `json:"origin,omitempty"`
	Destination     string    `json:"destination,omitempty"`
	DepartureTime   string    `json:"departure_time,omitempty"`
	ArrivalTime     string    `json:"arrival_time,omitempty"`
	DurationMinutes int       `json:"duration_minutes,omitempty"`
	AircraftType    string    `json:"aircraft_type,omitempty"`
	CabinCode       string    `json:"cabin_code,omitempty"`
	CabinName       string    `json:"cabin_name,omitempty"`
	BasePrice       float64   `json:"base_price,omitempty"`
	TaxPrice        float64   `json:"tax_price,omitempty"`
	TotalPrice      float64   `json:"total_price,omitempty"`
	Currency        string    `json:"currency,omitempty"`
	Availability    string    `json:"availability,omitempty"`
	RawProductCode  string    `json:"raw_product_code,omitempty"`
	RawPriceSource  string    `json:"raw_price_source,omitempty"`
	ObservedAt      time.Time `json:"observed_at"`
}

func LowestTotalPrice(observations []PriceObservation) (PriceObservation, bool) {
	var lowest PriceObservation
	for _, observation := range observations {
		if observation.TotalPrice <= 0 {
			continue
		}
		if lowest.TotalPrice == 0 || observation.TotalPrice < lowest.TotalPrice {
			lowest = observation
		}
	}
	return lowest, lowest.TotalPrice > 0
}
