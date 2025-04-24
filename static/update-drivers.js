// Global variable to store all drivers for filtering
let allDrivers = [];

// Status colors for styling
const statusColors = {
    'Available': '#4CAF50',
    'Busy': '#FF9800',
    'Offline': '#9E9E9E',
    'default': '#FFDE00'
};

// Update drivers on the map
function updateDrivers(data) {
    // Update driver count
    driverCountElement.textContent = `${data.count} Drivers`;

    // Update center location if provided
    let locationChanged = false;
    if (data.center) {
        // Only update if using city or if center has changed significantly
        if (selectedCity ||
            Math.abs(data.center.lat - userLocation.lat) > 0.001 ||
            Math.abs(data.center.lon - userLocation.lon) > 0.001) {

            userLocation.lat = data.center.lat;
            userLocation.lon = data.center.lon;
            locationChanged = true;
        }
    }

    // Update search radius if provided
    let radiusChanged = false;
    if (data.radius && Math.abs(data.radius - searchRadius) > 0.001) {
        searchRadius = data.radius;
        radiusInput.value = searchRadius;
        radiusChanged = true;

        // Update radius display text
        radiusValueElement.textContent = searchRadius.toFixed(2);
        const radiusKm = (searchRadius * 111.0).toFixed(1);
        radiusKmElement.textContent = radiusKm;
    }

    // Update search circle if location or radius changed
    if ((locationChanged || radiusChanged) && searchCircle) {
        // Remove old circle and create a new one for better rendering
        map.removeLayer(searchCircle);

        // Convert degrees to meters (more accurate conversion)
        const radiusMeters = searchRadius * 111000; // 111km per degree at equator

        // Create new circle
        searchCircle = L.circle([userLocation.lat, userLocation.lon], {
            color: 'blue',
            fillColor: '#30f',
            fillOpacity: 0.1,
            radius: radiusMeters,
            weight: 2,
            interactive: false,
            pane: 'overlayPane',
            className: 'no-pointer-events'
        }).addTo(map);
    }

    // Store all drivers for filtering
    allDrivers = data.drivers;

    // Update driver list in sidebar
    updateDriverList();

    // Count drivers by status
    const statusCounts = {
        'Available': 0,
        'Busy': 0,
        'Offline': 0
    };

    // Track which markers were updated
    const updatedMarkers = {};

    // Process drivers in batches for better performance
    const batchSize = 50;
    const totalDrivers = data.drivers.length;
    const drivers = data.drivers;

    // Process drivers in batches
    for (let i = 0; i < totalDrivers; i += batchSize) {
        const batch = drivers.slice(i, i + batchSize);

        // Process this batch
        batch.forEach(driver => {
            // Count by status
            if (statusCounts[driver.status] !== undefined) {
                statusCounts[driver.status]++;
            }

            const markerId = `driver-${driver.id}`;
            updatedMarkers[markerId] = true;

            if (markers[markerId]) {
                // Marker exists, update its position
                const prev = {
                    lat: markers[markerId].getLatLng().lat,
                    lon: markers[markerId].getLatLng().lng
                };

                // Store position history for detecting jumps
                if (!markers[markerId]._positionHistory) {
                    markers[markerId]._positionHistory = [];
                }

                // Add current position to history (keep last 3 positions)
                markers[markerId]._positionHistory.push({lat: driver.lat, lon: driver.lon});
                if (markers[markerId]._positionHistory.length > 3) {
                    markers[markerId]._positionHistory.shift();
                }

                // Calculate distance between current and previous position
                const distance = Math.sqrt(
                    Math.pow(driver.lat - prev.lat, 2) +
                    Math.pow(driver.lon - prev.lon, 2)
                );

                // Detect jumps (sudden large movements)
                let isJump = false;
                if (markers[markerId]._positionHistory.length >= 3) {
                    const positions = markers[markerId]._positionHistory;
                    const prevDistance = Math.sqrt(
                        Math.pow(positions[1].lat - positions[0].lat, 2) +
                        Math.pow(positions[1].lon - positions[0].lon, 2)
                    );

                    // If current distance is much larger than previous distance, it's a jump
                    if (distance > prevDistance * 5 && distance > 0.0005) {
                        isJump = true;
                    }
                }

                // Skip animation if the distance is too small (prevents jitter) or if it's a jump
                if (distance < 0.00001 || isJump) {
                    // For jumps, use a fade-out/fade-in effect
                    if (isJump) {
                        // Fade out
                        markers[markerId]._icon.style.transition = 'opacity 200ms ease-in-out';
                        markers[markerId]._icon.style.opacity = '0';

                        // After fade out, move and fade in
                        setTimeout(() => {
                            markers[markerId].setLatLng([driver.lat, driver.lon]);
                            markers[markerId]._icon.style.opacity = '1';
                        }, 200);
                    } else {
                        // Just set position for tiny movements
                        markers[markerId].setLatLng([driver.lat, driver.lon]);
                    }
                } else {
                    // Store speed in marker for smoothing
                    if (!markers[markerId]._speedHistory) {
                        markers[markerId]._speedHistory = [];
                    }

                    // Add current speed to history (keep last 5 values)
                    markers[markerId]._speedHistory.push(driver.speed);
                    if (markers[markerId]._speedHistory.length > 5) {
                        markers[markerId]._speedHistory.shift();
                    }

                    // Calculate average speed for smoother acceleration/deceleration
                    const avgSpeed = markers[markerId]._speedHistory.reduce((sum, speed) => sum + speed, 0) /
                                    markers[markerId]._speedHistory.length;

                    // Calculate animation duration based on smoothed speed
                    // Faster cars = shorter animation duration
                    // Speed is in degrees per second, we want duration in milliseconds
                    let duration = 200; // default for WebSocket

                    if (avgSpeed > 0) {
                        // Calculate real-world duration in ms
                        const realDuration = (distance / avgSpeed) * 1000;

                        // Clamp between 150ms and 300ms for smoother animation
                        // Longer durations create smoother movement
                        duration = Math.min(300, Math.max(150, realDuration));
                    }

                    // Choose easing based on speed change
                    let easing = 'easeInOutCubic'; // default

                    // Determine if accelerating, decelerating, or constant speed
                    if (markers[markerId]._speedHistory.length >= 2) {
                        const prevSpeed = markers[markerId]._speedHistory[markers[markerId]._speedHistory.length - 2];
                        const speedDiff = driver.speed - prevSpeed;

                        if (speedDiff > 0.00001) {
                            easing = 'easeInOutQuad'; // accelerating
                        } else if (speedDiff < -0.00001) {
                            easing = 'easeInOutCubic'; // decelerating (more pronounced)
                        } else {
                            easing = 'linear'; // constant speed
                        }
                    }

                    // Smooth transition to new position with appropriate easing
                    markers[markerId].slideTo([driver.lat, driver.lon], {
                        duration: duration,
                        keepAtCenter: false,
                        easing: easing
                    });
                }

                // Update rotation based on heading
                markers[markerId].setRotationAngle(driver.heading);

                // Create detailed popup content with driver information
                const popupContent = `
                    <div style="min-width: 180px; padding: 5px;">
                        <h3 style="margin: 0 0 8px 0; color: #333; border-bottom: 1px solid #ddd; padding-bottom: 5px;">
                            Taxi #${driver.id}
                        </h3>
                        <div style="margin-bottom: 5px;">
                            <strong>Status:</strong>
                            <span style="color:${statusColors[driver.status]}; font-weight: bold;">
                                ${driver.status}
                            </span>
                        </div>
                        <div style="margin-bottom: 5px;">
                            <strong>Distance:</strong> ${driver.distance.toFixed(2)} km
                        </div>
                        <div style="margin-bottom: 5px;">
                            <strong>Heading:</strong> ${driver.heading.toFixed(1)}°
                            <span style="color: #666; font-size: 0.9em;">
                                ${getDirectionFromHeading(driver.heading)}
                            </span>
                        </div>
                        <div style="margin-bottom: 5px;">
                            <strong>Speed:</strong> ${(driver.speed * 111.0 * 3600).toFixed(1)} km/h
                        </div>
                        <div style="margin-bottom: 5px;">
                            <strong>Coordinates:</strong><br>
                            ${driver.lat.toFixed(6)}, ${driver.lon.toFixed(6)}
                        </div>
                    </div>
                `;

                // Update popup content only if popup is open (performance optimization)
                if (markers[markerId].getPopup() && markers[markerId].getPopup().isOpen()) {
                    markers[markerId].getPopup().setContent(popupContent);
                } else if (!markers[markerId].getPopup()) {
                    // Create popup if it doesn't exist
                    markers[markerId].bindPopup(popupContent, {
                        offset: [0, -16],
                        closeButton: true,
                        autoClose: false,
                        className: 'driver-popup',
                        autoPan: false
                    });

                    // Add click event listener to ensure popup opens
                    markers[markerId].on('click', function() {
                        this.openPopup();
                    });
                }

                // Ensure marker is interactive and has proper z-index
                markers[markerId].setZIndexOffset(1000);

                // Update icon if status has changed
                const currentIcon = markers[markerId].options.icon;
                const newIcon = carIcons[driver.status] || carIcons['default'];

                // Check if we need to update the icon (status changed)
                if (currentIcon.options.iconUrl !== newIcon.options.iconUrl) {
                    markers[markerId].setIcon(newIcon);
                }
            } else {
                // Create new marker
                const icon = carIcons[driver.status] || carIcons['default'];

                // Create new marker with the appropriate icon
                const marker = L.marker([driver.lat, driver.lon], {
                    icon: icon,
                    // Ensure marker is interactive
                    interactive: true,
                    // Disable auto-panning which can cause jumping
                    autoPan: false,
                    // Set higher z-index to ensure it's clickable
                    zIndexOffset: 1000
                }).addTo(map);

                // Set initial rotation based on backend heading
                marker.setRotationAngle(driver.heading);

                // Create detailed popup content with driver information
                const popupContent = `
                    <div style="min-width: 180px; padding: 5px;">
                        <h3 style="margin: 0 0 8px 0; color: #333; border-bottom: 1px solid #ddd; padding-bottom: 5px;">
                            Taxi #${driver.id}
                        </h3>
                        <div style="margin-bottom: 5px;">
                            <strong>Status:</strong>
                            <span style="color:${statusColors[driver.status]}; font-weight: bold;">
                                ${driver.status}
                            </span>
                        </div>
                        <div style="margin-bottom: 5px;">
                            <strong>Distance:</strong> ${driver.distance.toFixed(2)} km
                        </div>
                        <div style="margin-bottom: 5px;">
                            <strong>Heading:</strong> ${driver.heading.toFixed(1)}°
                            <span style="color: #666; font-size: 0.9em;">
                                ${getDirectionFromHeading(driver.heading)}
                            </span>
                        </div>
                        <div style="margin-bottom: 5px;">
                            <strong>Speed:</strong> ${(driver.speed * 111.0 * 3600).toFixed(1)} km/h
                        </div>
                        <div style="margin-bottom: 5px;">
                            <strong>Coordinates:</strong><br>
                            ${driver.lat.toFixed(6)}, ${driver.lon.toFixed(6)}
                        </div>
                    </div>
                `;

                // Make popup open on click (not hover) - lazy binding for performance
                marker.on('click', function() {
                    // Only create popup when clicked
                    if (!this.getPopup()) {
                        this.bindPopup(popupContent, {
                            offset: [0, -16],
                            closeButton: true,
                            autoClose: false,
                            className: 'driver-popup',
                            autoPan: false
                        }).openPopup();
                    } else {
                        this.openPopup();
                    }
                });

                // Store marker
                markers[markerId] = marker;
            }
        });
    }

    // Remove markers that are no longer in the data
    Object.keys(markers).forEach(markerId => {
        if (!updatedMarkers[markerId]) {
            map.removeLayer(markers[markerId]);
            delete markers[markerId];
        }
    });

    // Update status counts in UI
    document.getElementById('available-count').textContent = statusCounts['Available'];
    document.getElementById('busy-count').textContent = statusCounts['Busy'];
    document.getElementById('offline-count').textContent = statusCounts['Offline'];
}

// Function to update the driver list in the sidebar
function updateDriverList() {
    if (!allDrivers) {
        return;
    }

    const driverListElement = document.getElementById('driver-list');
    if (!driverListElement) {
        return;
    }

    const statusFilter = document.getElementById('status-filter')?.value || 'all';
    const searchQuery = document.getElementById('driver-search')?.value?.toLowerCase() || '';

    // Clear the current list
    driverListElement.innerHTML = '';

    // Filter drivers based on status and search query
    console.log(`Filtering drivers with status filter: ${statusFilter}, search query: ${searchQuery}`);

    const filteredDrivers = allDrivers.filter(driver => {
        // Filter by status
        if (statusFilter !== 'all' && driver.status !== statusFilter) {
            return false;
        }

        // Filter by search query (driver ID)
        if (searchQuery && !`Taxi #${driver.id}`.toLowerCase().includes(searchQuery)) {
            return false;
        }

        return true;
    });

    // Sort drivers by ID
    filteredDrivers.sort((a, b) => a.id - b.id);

    filteredDrivers.forEach((driver, index) => {
        if (index < 5) {
            console.log(`Creating item for driver #${driver.id} with status ${driver.status}`);
        }

        const driverItem = document.createElement('div');
        driverItem.className = `driver-item driver-status-${driver.status.toLowerCase()}`;
        driverItem.dataset.id = driver.id;

        // Calculate speed in km/h
        const speedKmh = (driver.speed * 111.0 * 3600).toFixed(1);

        // Get direction from heading
        const direction = getDirectionFromHeading ? getDirectionFromHeading(driver.heading) : "N/A";

        driverItem.innerHTML = `
            <div class="driver-icon" style="background-image: url('/car-icon-${driver.status.toLowerCase()}.svg')"></div>
            <div class="driver-info">
                <div class="driver-id">Taxi #${driver.id}</div>
                <div class="driver-status">
                    <span class="driver-status-dot"></span>
                    ${driver.status}
                </div>
                <div class="driver-details">
                    <span class="driver-speed">${speedKmh} km/h</span>
                    <span class="driver-direction">${direction}</span>
                    <span class="driver-distance">${driver.distance.toFixed(1)} km</span>
                </div>
            </div>
        `;

        // Add click event to highlight on map
        driverItem.addEventListener('click', () => {
            // Remove selected class from all items
            document.querySelectorAll('.driver-item').forEach(item => {
                item.classList.remove('selected');
            });

            // Add selected class to clicked item
            driverItem.classList.add('selected');

            // Find the marker and open its popup
            const markerId = `driver-${driver.id}`;
            if (markers[markerId]) {
                // Pan to the marker
                map.panTo(markers[markerId].getLatLng());

                // Open the popup
                markers[markerId].openPopup();

                // Add a temporary highlight effect to the marker
                if (markers[markerId]._icon) {
                    markers[markerId]._icon.classList.add('highlight-marker');
                    setTimeout(() => {
                        markers[markerId]._icon.classList.remove('highlight-marker');
                    }, 1500);
                }
            }
        });

        driverListElement.appendChild(driverItem);
    });

    // Show message if no drivers match the filters
    if (filteredDrivers.length === 0) {
        const noDriversMessage = document.createElement('div');
        noDriversMessage.className = 'no-drivers-message';
        noDriversMessage.textContent = 'No drivers match your filters';
        driverListElement.appendChild(noDriversMessage);
    }
}

// Initialize sidebar functionality
function initializeSidebar() {
    // Toggle sidebar on mobile and desktop
    const toggleSidebarButton = document.getElementById('toggle-sidebar');
    const sidebar = document.querySelector('.sidebar');

    // Set initial state
    let sidebarVisible = true;

    toggleSidebarButton.addEventListener('click', () => {
        sidebarVisible = !sidebarVisible;

        if (sidebarVisible) {
            // Show sidebar
            sidebar.style.display = 'flex';
            setTimeout(() => {
                sidebar.style.transform = 'translateX(0)';
            }, 10);
            toggleSidebarButton.innerHTML = '<i class="icon-menu"></i>';
            toggleSidebarButton.setAttribute('title', 'Hide Sidebar');
        } else {
            // Hide sidebar
            sidebar.style.transform = 'translateX(-100%)';
            setTimeout(() => {
                sidebar.style.display = 'none';
            }, 300); // Match transition duration
            toggleSidebarButton.innerHTML = '<i class="icon-menu" style="transform: rotate(180deg);"></i>';
            toggleSidebarButton.setAttribute('title', 'Show Sidebar');
        }

        // Trigger a resize event to update the map
        window.dispatchEvent(new Event('resize'));
    });

    // Initialize status filter
    const statusFilter = document.getElementById('status-filter');
    statusFilter.addEventListener('change', updateDriverList);

    // Initialize driver search
    const driverSearch = document.getElementById('driver-search');
    driverSearch.addEventListener('input', updateDriverList);

    // Add CSS for highlight effect
    const style = document.createElement('style');
    style.textContent = `
        .highlight-marker {
            animation: pulse 1.5s ease-in-out;
            z-index: 1001 !important;
        }

        @keyframes pulse {
            0% { transform: scale(1); filter: brightness(1); }
            50% { transform: scale(1.3); filter: brightness(1.5); }
            100% { transform: scale(1); filter: brightness(1); }
        }

        .no-drivers-message {
            padding: 20px;
            text-align: center;
            color: #6c757d;
            font-style: italic;
        }
    `;
    document.head.appendChild(style);
}

// Helper function to convert heading in degrees to cardinal direction
function getDirectionFromHeading(heading) {
    // Normalize heading to 0-360 range
    while (heading < 0) heading += 360;
    while (heading >= 360) heading -= 360;

    // Define direction ranges
    const directions = [
        { name: "N", min: 337.5, max: 360 },
        { name: "N", min: 0, max: 22.5 },
        { name: "NE", min: 22.5, max: 67.5 },
        { name: "E", min: 67.5, max: 112.5 },
        { name: "SE", min: 112.5, max: 157.5 },
        { name: "S", min: 157.5, max: 202.5 },
        { name: "SW", min: 202.5, max: 247.5 },
        { name: "W", min: 247.5, max: 292.5 },
        { name: "NW", min: 292.5, max: 337.5 }
    ];

    // Find matching direction
    for (const dir of directions) {
        if ((dir.min <= heading && heading < dir.max) ||
            (dir.name === "N" && heading >= 337.5)) {
            return dir.name;
        }
    }

    return "N/A"; // Fallback
}

// Call this function when the page loads
document.addEventListener('DOMContentLoaded', initializeSidebar);
