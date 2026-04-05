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
	centerY := float64(height) / 2.0

	// --- Draw pass-through lane lines ---
	// These are vertical lines for lanes that have a branch/commit
	// passing through this row without a node here.
	for _, lane := range c.ActiveLanes {
		if lane == c.Lane {
			continue // The commit's own lane is drawn by edges/node.
		}
		x := float64(lane)*laneWidth + laneWidth/2.0
		color := laneColor(lane)
		cr.SetSourceRGB(color[0], color[1], color[2])
		cr.SetLineWidth(1.5)
		cr.SetDash(nil, 0)
		cr.MoveTo(x, 0)
		cr.LineTo(x, float64(height))
		cr.Stroke()
	}

	// --- Draw edges (lines from this commit to its parents) ---
	for _, edge := range c.Edges {
		drawEdge(cr, edge, centerY, height)
	}

	// --- Draw the commit node ---
	nodeX := float64(c.Lane)*laneWidth + laneWidth/2.0
	color := laneColor(c.Lane)

	if c.IsMerge {
		// Merge commits are drawn as diamonds.
		drawDiamond(cr, nodeX, centerY, nodeRadius+1, color)
	} else {
		// Regular commits are circles.
		drawCircle(cr, nodeX, centerY, nodeRadius, color)
	}

	// HEAD commit gets a double-ring.
	if c.IsHead {
		cr.SetSourceRGB(color[0], color[1], color[2])
		cr.SetLineWidth(1.5)
		cr.Arc(nodeX, centerY, nodeRadius+3, 0, 2*math.Pi)
		cr.Stroke()
	}
}

// drawEdge draws a connection line from this commit to a parent.
func drawEdge(cr *cairo.Context, edge git.GraphEdge, centerY float64, height int) {
	fromX := float64(edge.FromLane)*laneWidth + laneWidth/2.0
	toX := float64(edge.ToLane)*laneWidth + laneWidth/2.0
	color := laneColor(edge.ToLane)

	cr.SetSourceRGB(color[0], color[1], color[2])

	if edge.Style == git.EdgeDashed {
		cr.SetDash([]float64{4, 4}, 0)
	} else {
		cr.SetDash(nil, 0)
	}

	cr.SetLineWidth(1.5)

	if edge.FromLane == edge.ToLane {
		// Straight vertical line — the edge stays in the same lane.
		cr.MoveTo(fromX, 0)
		cr.LineTo(fromX, float64(height))
	} else {
		// Curved line — the edge crosses lanes.
		// Use a bezier curve for smooth visual appearance.
		cr.MoveTo(fromX, centerY)
		// Control points create a smooth S-curve.
		cp1y := centerY + float64(height)/4.0
		cp2y := float64(height) - float64(height)/4.0
		cr.CurveTo(fromX, cp1y, toX, cp2y, toX, float64(height))
	}

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
