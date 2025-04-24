## What is a Quadtree?
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

### Visual Representation:
```
Initial Space (before any splits):
+---------------------+
|                     |
|                     |
|                     |
|                     |
|                     |
|                     |
|                     |
+---------------------+

After first subdivision:
+---------------------+
|         |           |
|    NW   |    NE     |
|         |           |
|---------+-----------|
|         |           |
|    SW   |    SE     |
|         |           |
+---------------------+

After further subdivisions:
+---------------------+
| NW |    |           |
|----+ NE |           |
| NW |    |           |
|---------+-----------|
|    | SE |           |
| SW |----|           |
|    | SE |           |
+---------------------+
```