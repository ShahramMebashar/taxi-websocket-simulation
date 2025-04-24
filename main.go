package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"quadtree/quadtree"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// World bounds (longitude/latitude) - focused on Erbil and Duhok
	minLon, minLat = 42.5, 35.5 // Southwest corner
	maxLon, maxLat = 44.5, 37.5 // Northeast corner

	// Simulation parameters
	numDrivers        = 1000                   // 1,000 drivers
	searchRadius      = 0.15                   // degrees (approximately 16.5km at equator)
	maxSpeed          = 0.0001                 // degrees per second (about 11m/s or 40km/h) - increased for visibility
	minSpeed          = 0.00005                // minimum speed (about 5.5m/s or 20km/h) - increased for visibility
	updateInterval    = 220 * time.Millisecond // Reduced update frequency by 10% (from 200ms to 220ms)
	statsInterval     = 5 * time.Second
	queryInterval     = 2 * time.Second
	driverStatusProbs = 0.7 // 70% available, 30% will be busy or offline

	// Movement parameters for more realistic behavior
	turnProbability  = 0.05 // Increased probability of changing direction for more dynamic movement
	turnMaxAngle     = 0.15 // Slightly larger turn angle (about 8.6 degrees)
	accelerationProb = 0.05 // Increased probability of changing speed
	accelerationMax  = 0.15 // Larger acceleration/deceleration factor

	// City centers to cluster drivers around - we'll use only Erbil and Duhok
	numCities = 2

	// Server settings
	serverPort = 8080
)

// DriverStatus represents the current status of a driver
type DriverStatus int

const (
	Available DriverStatus = iota
	Busy
	Offline
)

func (s DriverStatus) String() string {
	switch s {
	case Available:
		return "Available"
	case Busy:
		return "Busy"
	case Offline:
		return "Offline"
	default:
		return "Unknown"
	}
}

// Driver represents a driver with an ID, location, and status
type Driver struct {
	ID      int          `json:"id"`
	Lon     float64      `json:"lon"`
	Lat     float64      `json:"lat"`
	Status  DriverStatus `json:"status"`
	Speed   float64      `json:"speed"`
	Heading float64      `json:"heading"` // in radians
	mu      sync.Mutex   `json:"-"`
}

// DriverResponse is the JSON response format for driver data
type DriverResponse struct {
	ID       int     `json:"id"`
	Lon      float64 `json:"lon"`
	Lat      float64 `json:"lat"`
	Status   string  `json:"status"`
	Distance float64 `json:"distance,omitempty"` // distance in km from query point
	Heading  float64 `json:"heading"`            // direction in degrees (0-360)
	Speed    float64 `json:"speed"`              // speed in degrees per second
}

// DriversResponse is the JSON response format for multiple drivers
type DriversResponse struct {
	Drivers []DriverResponse `json:"drivers"`
	Count   int              `json:"count"`
	Center  struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"center"`
	Radius float64 `json:"radius"`
}

// City represents a city center where drivers tend to cluster
type City struct {
	Name     string
	Lon, Lat float64
	Radius   float64 // in degrees
}

// Move updates the driver's position based on speed and heading
// Now with smoother, more realistic movement
func (d *Driver) Move(deltaTime float64, r *rand.Rand) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Only move if the driver is available or busy
	if d.Status == Offline {
		return
	}

	// Gradually change heading (smoother turns)
	if r.Float64() < turnProbability {
		// Small, gradual turns (more realistic)
		turnAmount := (r.Float64()*2 - 1.0) * turnMaxAngle
		d.Heading += turnAmount

		// Keep heading in [0, 2π] range
		if d.Heading < 0 {
			d.Heading += 2 * math.Pi
		} else if d.Heading > 2*math.Pi {
			d.Heading -= 2 * math.Pi
		}
	}

	// Gradually change speed (acceleration/deceleration)
	if r.Float64() < accelerationProb {
		// Change speed by up to ±20%
		speedChange := 1.0 + (r.Float64()*2-1.0)*accelerationMax
		d.Speed *= speedChange

		// Keep speed within limits
		if d.Speed < minSpeed {
			d.Speed = minSpeed
		} else if d.Speed > maxSpeed {
			d.Speed = maxSpeed
		}
	}

	// Calculate new position
	deltaLon := math.Sin(d.Heading) * d.Speed * deltaTime
	deltaLat := math.Cos(d.Heading) * d.Speed * deltaTime

	newLon := d.Lon + deltaLon
	newLat := d.Lat + deltaLat

	// Check if we're approaching a boundary and adjust heading to avoid it
	// This creates more natural movement near boundaries
	boundaryBuffer := 0.01 // Buffer zone near boundaries

	if newLon < minLon+boundaryBuffer {
		// Approaching west boundary, turn east
		d.Heading = r.Float64() * math.Pi
	} else if newLon > maxLon-boundaryBuffer {
		// Approaching east boundary, turn west
		d.Heading = math.Pi + r.Float64()*math.Pi
	}

	if newLat < minLat+boundaryBuffer {
		// Approaching south boundary, turn north
		d.Heading = math.Pi*1.5 + r.Float64()*math.Pi
	} else if newLat > maxLat-boundaryBuffer {
		// Approaching north boundary, turn south
		d.Heading = r.Float64() * math.Pi
	}

	// Recalculate position after potential heading change
	deltaLon = math.Sin(d.Heading) * d.Speed * deltaTime
	deltaLat = math.Cos(d.Heading) * d.Speed * deltaTime

	newLon = d.Lon + deltaLon
	newLat = d.Lat + deltaLat

	// Ensure we stay within bounds
	if newLon < minLon {
		newLon = minLon
	} else if newLon > maxLon {
		newLon = maxLon
	}

	if newLat < minLat {
		newLat = minLat
	} else if newLat > maxLat {
		newLat = maxLat
	}

	d.Lon = newLon
	d.Lat = newLat

	// Randomly change status occasionally (1% chance per update)
	if r.Float64() < 0.01 {
		statusRoll := r.Float64()
		if statusRoll < driverStatusProbs {
			d.Status = Available
		} else if statusRoll < driverStatusProbs+0.2 {
			d.Status = Busy
		} else {
			d.Status = Offline
		}
	}
}

// GetPosition returns the current position of the driver
func (d *Driver) GetPosition() (float64, float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.Lon, d.Lat
}

// GetStatus returns the current status of the driver
func (d *Driver) GetStatus() DriverStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.Status
}

// WebSocketClient represents a connected client
type WebSocketClient struct {
	conn     *websocket.Conn
	clientID string
	// Client parameters
	lat    float64
	lon    float64
	radius float64
	city   string
	// Mutex to prevent concurrent writes
	mu *sync.Mutex
}

// Simulation represents the entire driver simulation
type Simulation struct {
	drivers      []*Driver
	cities       []City
	quadtree     *quadtree.Quadtree
	quadtreeMu   sync.RWMutex
	stats        SimulationStats
	statsMu      sync.Mutex
	lastRebuild  time.Time
	rebuildCount int
	rand         *rand.Rand

	// WebSocket related fields
	clients   map[string]*WebSocketClient
	clientsMu sync.RWMutex
	upgrader  websocket.Upgrader
}

// SimulationStats tracks statistics about the simulation
type SimulationStats struct {
	TotalQueries       int
	TotalDriversFound  int
	AvgQueryTime       time.Duration
	AvgDriversPerQuery float64
	AvailableDrivers   int
	BusyDrivers        int
	OfflineDrivers     int
}

// NewSimulation creates a new driver simulation
func NewSimulation(r *rand.Rand) *Simulation {
	// Create cities
	cities := generateCities(numCities, r)

	// Create quadtree
	worldBounds := quadtree.Bounds{MinX: minLon, MinY: minLat, MaxX: maxLon, MaxY: maxLat}
	qt := quadtree.New(worldBounds, 8)

	// Create drivers
	drivers := make([]*Driver, numDrivers)
	for i := 0; i < numDrivers; i++ {
		// Always assign to a city - no random positions outside cities
		var lon, lat float64

		// All drivers in Erbil as per requirement
		cityIndex := 0 // Always Erbil
		city := cities[cityIndex]

		// Generate position within Erbil city center
		angle := r.Float64() * 2 * math.Pi
		// Use smaller radius to concentrate in city center (10-60% of city radius)
		// This ensures drivers are more visible and concentrated
		distance := (0.1 + r.Float64()*0.5) * city.Radius
		lon = city.Lon + math.Sin(angle)*distance
		lat = city.Lat + math.Cos(angle)*distance

		// Assign random status based on probability
		var status DriverStatus
		statusRoll := r.Float64()
		if statusRoll < driverStatusProbs {
			status = Available
		} else if statusRoll < driverStatusProbs+0.2 {
			status = Busy
		} else {
			status = Offline
		}

		// Create driver with realistic speed range
		drivers[i] = &Driver{
			ID:      i + 1,
			Lon:     lon,
			Lat:     lat,
			Status:  status,
			Speed:   minSpeed + r.Float64()*(maxSpeed-minSpeed), // Speed between min and max
			Heading: r.Float64() * 2 * math.Pi,
		}

		// Insert into quadtree
		qt.Insert(quadtree.Point{X: lon, Y: lat})
	}

	return &Simulation{
		drivers:     drivers,
		cities:      cities,
		quadtree:    qt,
		lastRebuild: time.Now(),
		rand:        r,

		// Initialize WebSocket related fields
		clients: make(map[string]*WebSocketClient),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for development
			},
		},
	}
}

// generateCities creates city centers for the simulation
// Now specifically for Erbil and Duhok
func generateCities(count int, r *rand.Rand) []City {
	// We'll ignore the count parameter and just create our two specific cities
	cities := make([]City, 2)

	// Erbil coordinates: approximately 36.191113 N, 44.009167 E
	cities[0] = City{
		Name:   "Erbil",
		Lat:    36.191113,
		Lon:    44.009167,
		Radius: 0.1, // About 11km radius
	}

	// Duhok coordinates: approximately 36.867905 N, 42.948857 E
	cities[1] = City{
		Name:   "Duhok",
		Lat:    36.867905,
		Lon:    42.948857,
		Radius: 0.08, // About 8.8km radius
	}

	return cities
}

// RebuildQuadtree rebuilds the quadtree with current driver positions
func (s *Simulation) RebuildQuadtree() {
	s.quadtreeMu.Lock()
	defer s.quadtreeMu.Unlock()

	// Create new quadtree
	worldBounds := quadtree.Bounds{MinX: minLon, MinY: minLat, MaxX: maxLon, MaxY: maxLat}
	qt := quadtree.New(worldBounds, 8)

	// Insert all drivers
	for _, driver := range s.drivers {
		lon, lat := driver.GetPosition()
		qt.Insert(quadtree.Point{X: lon, Y: lat})
	}

	s.quadtree = qt
	s.rebuildCount++
	s.lastRebuild = time.Now()
}

// UpdateStats updates the simulation statistics
func (s *Simulation) UpdateStats() {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	// Count drivers by status
	available, busy, offline := 0, 0, 0
	for _, driver := range s.drivers {
		status := driver.GetStatus()
		switch status {
		case Available:
			available++
		case Busy:
			busy++
		case Offline:
			offline++
		}
	}

	s.stats.AvailableDrivers = available
	s.stats.BusyDrivers = busy
	s.stats.OfflineDrivers = offline

	if s.stats.TotalQueries > 0 {
		s.stats.AvgDriversPerQuery = float64(s.stats.TotalDriversFound) / float64(s.stats.TotalQueries)
	}
}

// PrintStats prints the current simulation statistics
func (s *Simulation) PrintStats() {
	s.statsMu.Lock()
	stats := s.stats
	s.statsMu.Unlock()

	fmt.Printf("\n--- Simulation Statistics ---\n")
	fmt.Printf("Driver Status: %d Available, %d Busy, %d Offline\n",
		stats.AvailableDrivers, stats.BusyDrivers, stats.OfflineDrivers)
	fmt.Printf("Queries: %d total, %.2f drivers/query avg\n",
		stats.TotalQueries, stats.AvgDriversPerQuery)
	fmt.Printf("Average Query Time: %v\n", stats.AvgQueryTime)
	fmt.Printf("Quadtree Rebuilds: %d (last: %v ago)\n",
		s.rebuildCount, time.Since(s.lastRebuild).Round(time.Second))
	fmt.Printf("-----------------------------\n")
}

// QueryNearbyDrivers finds drivers near a given location
func (s *Simulation) QueryNearbyDrivers(lon, lat float64, radius float64) []quadtree.Point {
	s.quadtreeMu.RLock()
	defer s.quadtreeMu.RUnlock()

	// Create search bounds
	searchBounds := quadtree.Bounds{
		MinX: lon - radius,
		MinY: lat - radius,
		MaxX: lon + radius,
		MaxY: lat + radius,
	}

	// Query quadtree
	start := time.Now()
	nearbyPoints := s.quadtree.QueryResults(searchBounds)
	elapsed := time.Since(start)

	// Update stats
	s.statsMu.Lock()
	s.stats.TotalQueries++
	s.stats.TotalDriversFound += len(nearbyPoints)

	// Update average query time using weighted average
	if s.stats.TotalQueries == 1 {
		s.stats.AvgQueryTime = elapsed
	} else {
		weight := 0.1 // Weight for new value
		s.stats.AvgQueryTime = time.Duration(
			float64(s.stats.AvgQueryTime)*(1-weight) + float64(elapsed)*weight,
		)
	}
	s.statsMu.Unlock()

	return nearbyPoints
}

// Run starts the simulation
func (s *Simulation) Run() {
	// Set up channels for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	// Set up tickers for periodic events
	updateTicker := time.NewTicker(updateInterval)
	statsTicker := time.NewTicker(statsInterval)
	queryTicker := time.NewTicker(queryInterval)
	rebuildTicker := time.NewTicker(1 * time.Second)          // More frequent rebuilds for accurate quadtree
	broadcastTicker := time.NewTicker(220 * time.Millisecond) // Broadcast driver updates every 220ms (reduced by 10%)

	fmt.Println("Starting driver simulation with", numDrivers, "drivers")
	fmt.Println("Press Ctrl+C to stop the simulation")

	// Main simulation loop
	for {
		select {
		case <-stop:
			fmt.Println("\nStopping simulation...")
			updateTicker.Stop()
			statsTicker.Stop()
			queryTicker.Stop()
			rebuildTicker.Stop()
			broadcastTicker.Stop()
			return

		case <-updateTicker.C:
			// Update driver positions
			deltaTime := updateInterval.Seconds()
			for _, driver := range s.drivers {
				driver.Move(deltaTime, s.rand)
			}

		case <-statsTicker.C:
			// Update and print statistics
			s.UpdateStats()
			s.PrintStats()

		case <-queryTicker.C:
			// Simulate user queries
			userLon := minLon + s.rand.Float64()*(maxLon-minLon)
			userLat := minLat + s.rand.Float64()*(maxLat-minLat)

			// Find nearby city if any
			var nearestCity *City
			var minDist float64 = math.MaxFloat64

			for i, city := range s.cities {
				dist := distance(userLon, userLat, city.Lon, city.Lat)
				if dist < minDist {
					minDist = dist
					nearestCity = &s.cities[i]
				}
			}

			var locationDesc string
			if nearestCity != nil && minDist < nearestCity.Radius*2 {
				locationDesc = fmt.Sprintf("near %s", nearestCity.Name)
			} else {
				locationDesc = "in remote area"
			}

			fmt.Printf("\nUser %s at (%.6f, %.6f)\n", locationDesc, userLon, userLat)

			// Find nearby drivers
			nearbyPoints := s.QueryNearbyDrivers(userLon, userLat, searchRadius)

			fmt.Printf("Found %d drivers within %.2f degrees (≈%.1f km)\n",
				len(nearbyPoints), searchRadius, searchRadius*111.0)

			// Print first few drivers
			maxDisplay := 5
			if len(nearbyPoints) < maxDisplay {
				maxDisplay = len(nearbyPoints)
			}

			for j := 0; j < maxDisplay; j++ {
				point := nearbyPoints[j]
				dist := distance(userLon, userLat, point.X, point.Y)
				distKm := dist * 111.0 // Rough conversion to km

				// All drivers are Available for testing smoothness
				fmt.Printf("  Driver (Available) at (%.6f, %.6f), %.2f km away\n",
					point.X, point.Y, distKm)
			}

		case <-rebuildTicker.C:
			// Rebuild quadtree periodically
			s.RebuildQuadtree()

		case <-broadcastTicker.C:
			// Broadcast driver updates to all connected WebSocket clients
			s.BroadcastDrivers()
		}
	}
}

// distance calculates the Euclidean distance between two points
// This is a simplification; for real-world use, you'd want to use the haversine formula
func distance(lon1, lat1, lon2, lat2 float64) float64 {
	return math.Sqrt((lon2-lon1)*(lon2-lon1) + (lat2-lat1)*(lat2-lat1))
}

// HandleWebSocket handles WebSocket connections
func (s *Simulation) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP connection to WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}

	// Generate a unique client ID
	clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())

	// Create a new client
	client := &WebSocketClient{
		conn:     conn,
		clientID: clientID,
	}

	// Add client to the map
	s.clientsMu.Lock()
	s.clients[clientID] = client
	s.clientsMu.Unlock()

	log.Printf("New WebSocket client connected: %s", clientID)

	// Handle client disconnect
	defer func() {
		conn.Close()
		s.clientsMu.Lock()
		delete(s.clients, clientID)
		s.clientsMu.Unlock()
		log.Printf("WebSocket client disconnected: %s", clientID)
	}()

	// Keep the connection alive and handle client messages
	for {
		// Read message from client
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		// Process client messages
		if messageType == websocket.TextMessage {
			var clientParams map[string]interface{}
			if err := json.Unmarshal(message, &clientParams); err == nil {
				// Check if this is a client_params message
				if msgType, ok := clientParams["type"].(string); ok && msgType == "client_params" {
					// Update client parameters
					if lat, ok := clientParams["lat"].(float64); ok {
						client.lat = lat
					}
					if lon, ok := clientParams["lon"].(float64); ok {
						client.lon = lon
					}
					if radius, ok := clientParams["radius"].(float64); ok {
						client.radius = radius
					}
					if city, ok := clientParams["city"].(string); ok {
						client.city = city
					}

					log.Printf("Updated client %s parameters: lat=%.6f, lon=%.6f, radius=%.2f, city=%s",
						client.clientID, client.lat, client.lon, client.radius, client.city)

					// Send immediate update with the new parameters
					s.SendDriversToClient(client)
				}
			}
		}
	}
}

// SendDriversToClient sends driver updates to a specific client based on their parameters
func (s *Simulation) SendDriversToClient(client *WebSocketClient) {
	// Default to all drivers if no parameters are set
	if client.lat == 0 && client.lon == 0 && client.city == "" {
		// Use default parameters
		client.lat = s.cities[0].Lat // Default to Erbil
		client.lon = s.cities[0].Lon
		client.radius = searchRadius
	}

	// Resolve city name to coordinates if needed
	if client.city != "" {
		cityFound := false
		for _, city := range s.cities {
			if strings.EqualFold(city.Name, client.city) {
				client.lat = city.Lat
				client.lon = city.Lon
				cityFound = true
				break
			}
		}

		if !cityFound {
			// Default to Erbil if city not found
			client.lat = s.cities[0].Lat
			client.lon = s.cities[0].Lon
		}
	}

	// Use client's radius or default
	radius := client.radius
	if radius < 0.01 {
		// Ensure minimum radius is 0.01 degrees (about 1.1km)
		radius = searchRadius
		log.Printf("Client %s radius too small (%.4f), using default: %.2f",
			client.clientID, client.radius, radius)
	}

	// Query nearby drivers based on client parameters
	nearbyPoints := s.QueryNearbyDrivers(client.lon, client.lat, radius)

	// Prepare driver responses
	driverResponses := make([]DriverResponse, 0, len(nearbyPoints))

	// Add driver details
	for _, point := range nearbyPoints {
		// Find the driver by position
		for _, driver := range s.drivers {
			dLon, dLat := driver.GetPosition()
			if math.Abs(dLon-point.X) < 0.0001 && math.Abs(dLat-point.Y) < 0.0001 {
				// Calculate distance
				dist := distance(client.lon, client.lat, point.X, point.Y)
				distKm := dist * 111.0 // Rough conversion to km

				// Get driver's heading in degrees (convert from radians)
				headingDegrees := driver.Heading * 180 / math.Pi

				// Ensure heading is in 0-360 range
				for headingDegrees < 0 {
					headingDegrees += 360
				}
				for headingDegrees >= 360 {
					headingDegrees -= 360
				}

				// Add to response
				driverResponses = append(driverResponses, DriverResponse{
					ID:       driver.ID,
					Lon:      point.X,
					Lat:      point.Y,
					Status:   driver.Status.String(),
					Distance: distKm,
					Heading:  headingDegrees,
					Speed:    driver.Speed,
				})
				break
			}
		}
	}

	// Create the message to send
	message := map[string]interface{}{
		"type":    "drivers_update",
		"drivers": driverResponses,
		"count":   len(driverResponses),
		"center": map[string]float64{
			"lat": client.lat,
			"lon": client.lon,
		},
		"radius": radius,
		"time":   time.Now().UnixNano() / int64(time.Millisecond), // Timestamp in milliseconds
	}

	// Convert to JSON
	jsonMessage, err := json.Marshal(message)
	if err != nil {
		log.Println("Error marshaling driver updates for client:", err)
		return
	}

	// Add a mutex to the client to prevent concurrent writes
	if client.mu == nil {
		client.mu = &sync.Mutex{}
	}

	// Lock the client mutex before writing
	client.mu.Lock()
	defer client.mu.Unlock()

	// Send to the client
	err = client.conn.WriteMessage(websocket.TextMessage, jsonMessage)
	if err != nil {
		log.Printf("Error sending to client %s: %v", client.clientID, err)
	}
}

// BroadcastDrivers sends driver updates to all connected clients
func (s *Simulation) BroadcastDrivers() {
	// Send updates to each client based on their parameters
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for _, client := range s.clients {
		s.SendDriversToClient(client)
	}
}

// GetNearbyDriversHandler handles API requests for nearby drivers
func (s *Simulation) GetNearbyDriversHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()

	// Get location parameters
	latStr := query.Get("lat")
	lonStr := query.Get("lon")
	radiusStr := query.Get("radius")
	cityName := query.Get("city")

	// Default values
	lat, lon := 0.0, 0.0
	radius := searchRadius

	// If city is specified, use its coordinates
	if cityName != "" {
		cityFound := false
		for _, city := range s.cities {
			if strings.EqualFold(city.Name, cityName) {
				lat = city.Lat
				lon = city.Lon
				cityFound = true
				break
			}
		}

		if !cityFound {
			// Default to Erbil if city not found
			lat = s.cities[0].Lat
			lon = s.cities[0].Lon
		}
	} else {
		// Parse custom coordinates if provided
		if latStr != "" {
			if val, err := strconv.ParseFloat(latStr, 64); err == nil {
				lat = val
			}
		}

		if lonStr != "" {
			if val, err := strconv.ParseFloat(lonStr, 64); err == nil {
				lon = val
			}
		}
	}

	// Parse radius
	if radiusStr != "" {
		if val, err := strconv.ParseFloat(radiusStr, 64); err == nil {
			radius = val
		}
	}

	// Query nearby drivers
	nearbyPoints := s.QueryNearbyDrivers(lon, lat, radius)

	// Prepare response
	response := DriversResponse{
		Drivers: make([]DriverResponse, 0, len(nearbyPoints)),
		Count:   len(nearbyPoints),
		Center: struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
		}{
			Lat: lat,
			Lon: lon,
		},
		Radius: radius,
	}

	// Add driver details
	for _, point := range nearbyPoints {
		// Find the driver by position
		for _, driver := range s.drivers {
			dLon, dLat := driver.GetPosition()
			if math.Abs(dLon-point.X) < 0.0001 && math.Abs(dLat-point.Y) < 0.0001 {
				// Calculate distance
				dist := distance(lon, lat, point.X, point.Y)
				distKm := dist * 111.0 // Rough conversion to km

				// Add to response with heading and speed
				// Get driver's heading in degrees (convert from radians)
				headingDegrees := driver.Heading * 180 / math.Pi

				// Ensure heading is in 0-360 range
				for headingDegrees < 0 {
					headingDegrees += 360
				}
				for headingDegrees >= 360 {
					headingDegrees -= 360
				}

				// Convert speed from degrees/second to km/h for better understanding
				// (We'll keep the original speed value in the response)

				response.Drivers = append(response.Drivers, DriverResponse{
					ID:       driver.ID,
					Lon:      point.X,
					Lat:      point.Y,
					Status:   driver.Status.String(), // Use actual driver status
					Distance: distKm,
					Heading:  headingDegrees,
					Speed:    driver.Speed,
				})
				break
			}
		}
	}

	// Send JSON response
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Allow CORS
	json.NewEncoder(w).Encode(response)
}

// StartServer starts the HTTP server
func StartServer(sim *Simulation) {
	// Create a file server for static files
	fs := http.FileServer(http.Dir("static"))

	// Register API handlers
	http.HandleFunc("/api/drivers", sim.GetNearbyDriversHandler)

	// Register WebSocket handler
	http.HandleFunc("/ws", sim.HandleWebSocket)

	// Register static file handler
	http.Handle("/", fs)

	// Start server
	serverAddr := fmt.Sprintf(":%d", serverPort)
	log.Printf("Starting HTTP server on %s", serverAddr)
	go func() {
		if err := http.ListenAndServe(serverAddr, nil); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()
}

func main() {
	// Use the newer approach for random number generation
	// As of Go 1.20, rand.Seed is deprecated
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Create simulation
	sim := NewSimulation(r)

	// Create static directory if it doesn't exist
	if err := os.MkdirAll("static", 0755); err != nil {
		log.Fatalf("Failed to create static directory: %v", err)
	}

	// Start HTTP server
	StartServer(sim)

	// Run simulation
	sim.Run()
}
