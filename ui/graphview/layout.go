// Package graphview — layout.go provides layout calculation helpers for
// the visual graph view.
//
// The visual graph view draws the entire commit DAG on a single large
// GtkDrawingArea. To convert between the abstract graph model (lanes,
// rows) and actual pixel coordinates, we need layout helpers that know
// the current lane width, row height, and scroll position.
//
// This file provides three main capabilities:
//  1. Coordinate conversion: lane/row → pixel X/Y and vice versa.
//  2. Hit testing: given a mouse click at pixel (px, py), find which
//     commit node or ref pill was clicked.
//  3. Visible range: given the scroll position and viewport size,
//     compute which rows are currently visible (to skip drawing
//     off-screen rows for performance).
package graphview

// Layout holds the configurable parameters for converting between
// the abstract graph model and pixel coordinates. The visual graph
// view creates one Layout and updates it when the user zooms.
type Layout struct {
	// LaneWidth is the horizontal pixel spacing between lane centers.
	// Each lane is a vertical column in the graph where a branch line
	// can run. Typical value: 20 pixels.
	LaneWidth float64

	// RowHeight is the vertical pixel spacing between commit rows.
	// This changes when the user zooms in/out with Ctrl+scroll.
	// Range: MinRowHeight to MaxRowHeight (20 to 48 pixels).
	RowHeight float64

	// TextOffsetX is where the commit text starts, measured from the
	// left edge of the drawing area. It is placed to the right of the
	// widest possible graph lane area.
	// Computed as: (maxLane + 2) * LaneWidth
	TextOffsetX float64

	// RefPillHeight is the height of a ref label pill in pixels.
	RefPillHeight float64

	// RefPillPadding is the horizontal padding inside a ref pill.
	RefPillPadding float64
}

// MinRowHeight is the smallest allowed row height (most zoomed out).
const MinRowHeight = 20.0

// MaxRowHeight is the largest allowed row height (most zoomed in).
const MaxRowHeight = 48.0

// DefaultLaneWidth is the default horizontal spacing between lanes.
const DefaultLaneWidth = 20.0

// DefaultRowHeight is the default vertical spacing between rows.
const DefaultRowHeight = 28.0

// DefaultLayout returns a Layout with sensible default values.
// This is used when the graph view is first created, before the
// user has zoomed in or out.
func DefaultLayout() Layout {
	return Layout{
		LaneWidth:      DefaultLaneWidth,
		RowHeight:      DefaultRowHeight,
		TextOffsetX:    0, // Will be computed after graph data loads.
		RefPillHeight:  16,
		RefPillPadding: 6,
	}
}

// NodeX returns the horizontal pixel coordinate for the center of a
// commit node in the given lane.
//
// Example: lane 0 with LaneWidth=20 → X = 10 (centered in first lane).
func (l Layout) NodeX(lane int) float64 {
	return float64(lane)*l.LaneWidth + l.LaneWidth/2.0
}

// NodeY returns the vertical pixel coordinate for the center of a
// commit node at the given row index.
//
// Example: row 0 with RowHeight=28 → Y = 14 (centered in first row).
func (l Layout) NodeY(row int) float64 {
	return float64(row)*l.RowHeight + l.RowHeight/2.0
}

// RowTop returns the Y coordinate of the top edge of the given row.
func (l Layout) RowTop(row int) float64 {
	return float64(row) * l.RowHeight
}

// RowBottom returns the Y coordinate of the bottom edge of the given row.
func (l Layout) RowBottom(row int) float64 {
	return float64(row+1) * l.RowHeight
}

// TotalHeight returns the total height of the graph in pixels,
// given the number of commits (rows).
func (l Layout) TotalHeight(numRows int) float64 {
	return float64(numRows) * l.RowHeight
}

// TotalWidth returns a reasonable total width for the drawing area.
// It accounts for the graph lanes plus space for commit text.
// The minTextWidth parameter specifies the minimum width reserved
// for commit subject text (typically 600-800 pixels).
func (l Layout) TotalWidth(maxLane int, minTextWidth float64) float64 {
	graphWidth := float64(maxLane+2) * l.LaneWidth
	return graphWidth + minTextWidth
}

// UpdateTextOffset recalculates TextOffsetX based on the maximum lane
// used in the graph. Call this after loading new graph data.
//
// The text offset is placed 2 lanes to the right of the widest lane,
// giving some breathing room between the graph lines and the text.
func (l *Layout) UpdateTextOffset(maxLane int) {
	l.TextOffsetX = float64(maxLane+2) * l.LaneWidth
}

// RowAtY returns the row index for a given Y pixel coordinate.
// If the coordinate is above the first row, returns 0.
// The caller must clamp the result to the valid range [0, numRows-1].
func (l Layout) RowAtY(y float64) int {
	if y < 0 {
		return 0
	}
	return int(y / l.RowHeight)
}

// VisibleRows returns the range of row indices [first, last] that are
// visible given the current scroll position and viewport height.
//
// Parameters:
//   - scrollY: the current vertical scroll offset in pixels.
//   - viewportHeight: the height of the visible viewport in pixels.
//   - totalRows: the total number of rows in the graph.
//
// Returns:
//   - first: the index of the first visible row (inclusive).
//   - last: the index of the last visible row (inclusive).
//
// Both first and last are clamped to [0, totalRows-1]. If totalRows
// is 0, returns (0, 0).
func (l Layout) VisibleRows(scrollY, viewportHeight float64, totalRows int) (first, last int) {
	if totalRows == 0 {
		return 0, 0
	}

	// The first visible row is the one whose top edge is at or above scrollY.
	first = int(scrollY / l.RowHeight)
	if first < 0 {
		first = 0
	}

	// The last visible row is the one whose top edge is still within the viewport.
	// We add 1 extra row as a buffer for partially visible rows.
	last = int((scrollY + viewportHeight) / l.RowHeight)
	if last >= totalRows {
		last = totalRows - 1
	}

	return first, last
}

// HitTestResult describes what the user clicked on in the graph view.
// Only one of the fields will be set depending on what was hit.
type HitTestResult struct {
	// HitNode is true if the click landed on or near a commit node.
	HitNode bool

	// HitRef is true if the click landed on a ref pill label.
	HitRef bool

	// Row is the row index of the hit commit (-1 if nothing was hit).
	Row int

	// RefIndex is the index into the commit's Refs slice (-1 if no ref hit).
	RefIndex int
}

// HitTest determines what element (if any) is at the given pixel
// coordinates. It checks commit nodes and ref pills.
//
// Parameters:
//   - px, py: the pixel coordinates to test (relative to drawing area origin).
//   - nodes: the list of all GraphNodes (with pre-computed pixel positions).
//   - scrollY: the current vertical scroll offset.
//   - viewportHeight: the height of the visible viewport.
//
// Returns a HitTestResult describing what was hit.
func (l Layout) HitTest(px, py float64, nodes []GraphNode) HitTestResult {
	// Default: nothing hit.
	result := HitTestResult{Row: -1, RefIndex: -1}

	// Quick check: determine which row the Y coordinate falls in.
	row := l.RowAtY(py)
	if row < 0 || row >= len(nodes) {
		return result
	}

	node := nodes[row]

	// Check ref pills first (they are drawn on top of other elements).
	for i, pill := range node.RefPills {
		if pill.ContainsPoint(px, py) {
			result.HitNode = true
			result.HitRef = true
			result.Row = row
			result.RefIndex = i
			return result
		}
	}

	// Check if the click is near the commit node circle.
	// We use a generous hit radius (8px) so small nodes are easy to click.
	const hitRadius = 8.0
	dx := px - node.X
	dy := py - node.Y
	if dx*dx+dy*dy <= hitRadius*hitRadius {
		result.HitNode = true
		result.Row = row
		return result
	}

	// Even if not on the node circle, clicking anywhere in the row
	// selects that commit. This matches typical list behavior.
	result.HitNode = true
	result.Row = row
	return result
}

// ClampRowHeight constrains a row height value to the allowed zoom range.
func ClampRowHeight(h float64) float64 {
	if h < MinRowHeight {
		return MinRowHeight
	}
	if h > MaxRowHeight {
		return MaxRowHeight
	}
	return h
}
