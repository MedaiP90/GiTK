// Package commitlog — graphrenderer.go implements the custom GtkDrawingArea
// widget that renders the DAG graph column in the commit log table.
//
// Each row in the commit log table has a DrawingArea that draws:
//   - Vertical lane lines (showing the flow of branches).
//   - A commit node (circle, diamond for merges, double-ring for HEAD).
//   - Connecting lines to parent commits (bezier curves for lane changes).
//
// The renderer uses Cairo for drawing, which provides smooth anti-aliased
// vector graphics. Colors come from the GNOME palette to ensure they
// look good in both light and dark themes.
//
// Data is passed to each DrawingArea by resetting its draw function closure
// on every bind. This avoids the fragile sync.Map + native-pointer key
// approach and works reliably with GTK4's list-item recycling.
package commitlog

import (
	"math"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// laneWidth is the horizontal spacing between lane centers.
const laneWidth = 18

// nodeRadius is the radius of a commit node circle.
const nodeRadius = 4

// rowHeight is the vertical height of each commit row.
const rowHeight = 28

// laneColors are the GNOME palette colors used for different lanes.
// These are chosen to be visible in both light and dark themes.
// We use 8 distinct colors and cycle through them for lanes 0-7.
var laneColors = [][3]float64{
	{0.208, 0.518, 0.894}, // Blue  (@blue_3)
	{0.180, 0.761, 0.494}, // Green (@green_4)
	{0.918, 0.608, 0.184}, // Yellow (@yellow_4)
	{0.878, 0.322, 0.322}, // Red   (@red_3)
	{0.612, 0.431, 0.843}, // Purple (@purple_3)
	{0.337, 0.765, 0.827}, // Teal  (@teal_3)
	{0.890, 0.545, 0.286}, // Orange (@orange_3)
	{0.659, 0.659, 0.659}, // Grey   (@grey_3)
}

// NewGraphRenderer creates a new DrawingArea configured for graph rendering.
// It initially draws nothing; call SetGraphCommit to bind data and trigger a redraw.
func NewGraphRenderer() *gtk.DrawingArea {
	da := gtk.NewDrawingArea()
	// Initial draw function does nothing (empty cell).
	da.SetDrawFunc(func(area *gtk.DrawingArea, cr *cairo.Context, width, height int) {})
	return da
}

// SetGraphCommit binds commit data to a DrawingArea by replacing its draw
// function with a closure that captures the commit. This is called from the
// ColumnView's bind callback on every row recycle.
func SetGraphCommit(da *gtk.DrawingArea, commit git.GraphCommit) {
	da.SetDrawFunc(func(area *gtk.DrawingArea, cr *cairo.Context, width, height int) {
		drawGraph(cr, commit, width, height)
	})
	da.QueueDraw()
}

// ClearGraphCommit resets a recycled DrawingArea to the empty state.
func ClearGraphCommit(da *gtk.DrawingArea) {
	da.SetDrawFunc(func(area *gtk.DrawingArea, cr *cairo.Context, width, height int) {})
	da.QueueDraw()
}

// drawGraph renders the lane lines, edges, and commit node for one row.
func drawGraph(cr *cairo.Context, c git.GraphCommit, width, height int) {
	h := float64(height)
	centerY := h / 2.0

	// Build a set of lanes that have incoming edges at this row.
	// These lanes are being merged into the commit's lane via a Bezier
	// curve, so we must NOT draw a straight pass-through line for them.
	skipLanes := make(map[int]bool, len(c.IncomingEdges)+len(c.Edges))
	for _, edge := range c.IncomingEdges {
		skipLanes[edge.FromLane] = true
	}
	// Also skip lanes that are the target of an outgoing cross-lane edge
	// from this commit — the Bezier curve handles the visual connection.
	for _, edge := range c.Edges {
		if edge.FromLane != edge.ToLane {
			skipLanes[edge.ToLane] = true
		}
	}

	// --- Pass 1: Draw active lane pass-through lines ---
	// Skip the commit's own lane (handled in Pass 2) and lanes that
	// have an incoming/outgoing edge (handled by a Bezier curve).
	for _, lane := range c.ActiveLanes {
		if lane == c.Lane || skipLanes[lane] {
			continue
		}
		x := float64(lane)*laneWidth + laneWidth/2.0
		color := laneColor(lane)
		cr.SetSourceRGB(color[0], color[1], color[2])
		cr.SetLineWidth(1.5)
		cr.SetDash(nil, 0)
		cr.MoveTo(x, 0)
		cr.LineTo(x, h)
		cr.Stroke()
	}

	// --- Pass 2: Draw the commit's own lane vertical line ---
	hasParents := len(c.Edges) > 0
	nodeX := float64(c.Lane)*laneWidth + laneWidth/2.0
	ownColor := laneColor(c.Lane)

	cr.SetSourceRGB(ownColor[0], ownColor[1], ownColor[2])
	cr.SetLineWidth(1.5)
	cr.SetDash(nil, 0)

	if !hasParents {
		// Root commit: line from the node to the top of the row.
		cr.MoveTo(nodeX, centerY)
		cr.LineTo(nodeX, 0)
		cr.Stroke()
	} else if c.IsLaneTip {
		// Tip of a branch: line from center down to bottom only.
		cr.MoveTo(nodeX, centerY)
		cr.LineTo(nodeX, h)
		cr.Stroke()
	} else {
		// Normal commit: full-height line connects to rows above and below.
		cr.MoveTo(nodeX, 0)
		cr.LineTo(nodeX, h)
		cr.Stroke()
	}

	// --- Pass 3: Draw outgoing cross-lane edges (curves to parent lanes) ---
	for _, edge := range c.Edges {
		if edge.FromLane != edge.ToLane {
			drawOutgoingEdge(cr, edge, centerY, height)
		}
	}

	// --- Pass 4: Draw incoming edges from converging children ---
	// These are branches that diverge upward: curves from the child's lane
	// at the top of the row to the commit's node center.
	for _, edge := range c.IncomingEdges {
		drawIncomingEdge(cr, edge, centerY, height)
	}

	// --- Pass 5: Draw the commit node on top ---
	if c.IsMerge {
		drawDiamond(cr, nodeX, centerY, nodeRadius+1, ownColor)
	} else {
		drawCircle(cr, nodeX, centerY, nodeRadius, ownColor)
	}

	// HEAD commit gets a double-ring.
	if c.IsHead {
		cr.SetSourceRGB(ownColor[0], ownColor[1], ownColor[2])
		cr.SetLineWidth(1.5)
		cr.Arc(nodeX, centerY, nodeRadius+3, 0, 2*math.Pi)
		cr.Stroke()
	}
}

// drawOutgoingEdge draws a curved connection line from this commit's node
// (centerY) down to a parent's lane at the bottom of the row.
func drawOutgoingEdge(cr *cairo.Context, edge git.GraphEdge, centerY float64, height int) {
	fromX := float64(edge.FromLane)*laneWidth + laneWidth/2.0
	toX := float64(edge.ToLane)*laneWidth + laneWidth/2.0
	h := float64(height)
	color := laneColor(edge.ToLane)

	cr.SetSourceRGB(color[0], color[1], color[2])

	if edge.Style == git.EdgeDashed {
		cr.SetDash([]float64{4, 4}, 0)
	} else {
		cr.SetDash(nil, 0)
	}

	cr.SetLineWidth(1.5)

	// Bezier curve from the node center down to the bottom of the row.
	cr.MoveTo(fromX, centerY)
	cp1y := centerY + h/4.0
	cp2y := h - h/4.0
	cr.CurveTo(fromX, cp1y, toX, cp2y, toX, h)
	cr.Stroke()
}

// drawIncomingEdge draws a curved connection line from a child's lane at
// the top of the row to this commit's node (centerY). This shows branch
// divergence: where a child in a different lane connects to this parent.
func drawIncomingEdge(cr *cairo.Context, edge git.GraphEdge, centerY float64, height int) {
	fromX := float64(edge.FromLane)*laneWidth + laneWidth/2.0
	toX := float64(edge.ToLane)*laneWidth + laneWidth/2.0
	color := laneColor(edge.FromLane)

	cr.SetSourceRGB(color[0], color[1], color[2])

	if edge.Style == git.EdgeDashed {
		cr.SetDash([]float64{4, 4}, 0)
	} else {
		cr.SetDash(nil, 0)
	}

	cr.SetLineWidth(1.5)

	// Bezier curve from the top of the row to the node center.
	cp1y := centerY / 4.0
	cp2y := centerY - centerY/4.0
	cr.MoveTo(fromX, 0)
	cr.CurveTo(fromX, cp1y, toX, cp2y, toX, centerY)
	cr.Stroke()
}

// drawCircle draws a filled circle at the given position.
func drawCircle(cr *cairo.Context, x, y, radius float64, color [3]float64) {
	cr.SetSourceRGB(color[0], color[1], color[2])
	cr.Arc(x, y, radius, 0, 2*math.Pi)
	cr.Fill()
}

// drawDiamond draws a filled diamond (rotated square) at the given position.
// Used for merge commits to visually distinguish them from regular commits.
func drawDiamond(cr *cairo.Context, x, y, size float64, color [3]float64) {
	cr.SetSourceRGB(color[0], color[1], color[2])
	cr.MoveTo(x, y-size)
	cr.LineTo(x+size, y)
	cr.LineTo(x, y+size)
	cr.LineTo(x-size, y)
	cr.ClosePath()
	cr.Fill()
}

// laneColor returns the color for a given lane index, cycling through
// the palette.
func laneColor(lane int) [3]float64 {
	return laneColors[lane%len(laneColors)]
}
