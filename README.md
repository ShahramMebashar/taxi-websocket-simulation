# Driver Simulation and Visualization

A real-time driver simulation and visualization system using Go for the backend and JavaScript/Leaflet for the frontend. The system simulates 1,000 taxi drivers in Erbil and Duhok cities with different statuses (Available, Busy, Offline) and provides a web-based visualization.

![Driver Simulation Screenshot](screenshot.png)

*Screenshot: Driver simulation showing drivers with different statuses on the map and in the sidebar list.*

## Features

- **Real-time Driver Simulation**: Simulates 1,000 drivers with realistic movement patterns
- **WebSocket Communication**: Provides real-time updates to connected clients
- **Interactive Map Visualization**: Uses Leaflet.js with OpenStreetMap for visualization
- **Spatial Indexing**: Uses Quadtree for efficient spatial queries
- **Driver Status Management**: Drivers can be Available, Busy, or Offline
- **Customizable Search Radius**: Adjust the search radius to find nearby drivers
- **City Selection**: Focus on specific cities (Erbil or Duhok)
- **Responsive UI**: Sidebar with driver list, status filters, and search functionality

## Technical Implementation

### Backend (Go)

- **Quadtree Implementation**: Efficient spatial indexing for driver queries
- **WebSocket Server**: Real-time communication with clients
- **Driver Simulation**: Realistic movement patterns with heading and speed
- **RESTful API**: HTTP endpoints for driver data
- **Concurrent Processing**: Goroutines for simulation and client communication

### Frontend (JavaScript/HTML/CSS)

- **Leaflet.js Map**: Interactive map with custom markers
- **Real-time Updates**: WebSocket connection for live driver updates
- **Smooth Animations**: Interpolated driver movements with heading
- **Responsive Design**: Mobile-friendly interface with collapsible sidebar
- **Driver Filtering**: Filter drivers by status and search by ID
- **Custom Car Icons**: Different icons for each driver status

## How It Works

The system uses a Quadtree data structure for efficient spatial indexing, allowing quick queries for drivers within a specific radius. The backend simulates driver movement with realistic patterns, including:

- Random status changes (Available, Busy, Offline)
- Varying speeds and headings
- City-centered distribution

The frontend connects to the backend via WebSocket for real-time updates and visualizes the drivers on an interactive map with smooth animations.

### WebSocket Communication

The client and server communicate using a simple JSON protocol:

```json
// Client to Server (parameters)
{
  "type": "client_params",
  "lat": 36.191113,
  "lon": 44.009167,
  "radius": 0.15,
  "city": "Erbil"
}

// Server to Client (driver updates)
{
  "type": "drivers_update",
  "drivers": [
    {
      "id": 42,
      "lat": 36.191113,
      "lon": 44.009167,
      "status": "Available",
      "heading": 45.0,
      "speed": 0.00008,
      "distance": 2.5
    },
    // More drivers...
  ],
  "count": 1000,
  "center": {
    "lat": 36.191113,
    "lon": 44.009167
  },
  "radius": 0.15,
  "time": 1619712345678
}
```

The client can adjust the search parameters (location, radius, city) and the server responds with driver updates in real-time.

## Quadtree Implementation

A quadtree is a tree data structure where each internal node has exactly four children. It's used to partition a two-dimensional space by recursively subdividing it into four quadrants or regions.

### Key Properties:
- Space Partitioning: Divides space into adaptable cells
- Hierarchical: Organized as a tree with parent-child relationships
- Point Capacity: Each node holds a maximum number of points before splitting

### Common Use Cases:
- Spatial indexing (e.g., finding nearby points)
- Image processing
- Collision detection in games
- Geographic information systems (GIS)

## User Interface Components

The web interface consists of several key components:

### Sidebar
- **Status Summary**: Shows counts of Available, Busy, and Offline drivers
- **Controls**: City selection and search radius adjustment
- **Driver List**: Filterable list of drivers with status indicators
- **Search & Filter**: Find drivers by ID or filter by status

### Map
- **Interactive Map**: Pan and zoom to explore the area
- **Driver Markers**: Color-coded car icons showing driver status
- **Search Circle**: Visual indicator of the search radius
- **Popups**: Detailed driver information on click

### Interactions
- **Click on Map**: Set a new search location
- **Adjust Radius**: Slide to change the search radius
- **Select City**: Choose between Erbil and Duhok
- **Click on Driver**: Highlight and center on the map
- **Toggle Sidebar**: Collapse/expand for more map space

## Getting Started

1. Clone the repository
2. Run the Go server: `go run main.go`
3. Open a web browser and navigate to `http://localhost:8080`
4. Interact with the map to see drivers in real-time

## Requirements

- Go 1.16+
- Modern web browser with WebSocket support

## License

MIT License