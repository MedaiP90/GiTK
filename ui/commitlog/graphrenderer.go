// Package commitlog — graphrenderer.go implements the custom GtkDrawingArea
// widget that renders the DAG graph column in the commit log table.
//
// Each row in the commit log table has a GraphRenderer that draws:
//   - Vertical lane lines (showing the flow of branches).
//   - A commit node (circle, diamond for merges, double-ring for HEAD).
//   - Connecting lines to parent commits (bezier curves for lane changes).
//
// The renderer uses Cairo for drawing, which provides smooth anti-aliased
// vector graphics. Colors come from the GNOME palette to ensure they
// look good in both light and dark themes.
//
// Performance: This drawing function is called for every visible row on
// every frame. It must be fast — no allocations, no complex computation.
// All layout data is pre-computed in git.BuildGraph().
package commitlog

import (
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

// GraphRenderer is a GtkDrawingArea that renders one row of the DAG graph.
// It's embedded as a widget in each row of the GtkColumnView graph column.
type GraphRenderer struct {
	*gtk.DrawingArea

	// commit holds the graph layout data for this row.
	commit git.GraphCommit

	// hasCommit is true if SetCommit has been called.
	hasCommit bool
}

// NewGraphRenderer creates a new graph drawing area.
func NewGraphRenderer() *GraphRenderer {
	gr := &GraphRenderer{
		DrawingArea: gtk.NewDrawingArea(),
	}

	// Set up the draw function. GTK calls this whenever the widget
	// needs to be redrawn (e.g., when it becomes visible, or when
	// we call QueueDraw()).
	gr.SetDrawFunc(func(area *gtk.DrawingArea, cr *cairo.Context, width, height int) {
		gr.draw(cr, width, height)
	})

	return gr
}

// SetCommit sets the graph data for this row and triggers a redraw.
func (gr *GraphRenderer) SetCommit(commit git.GraphCommit) {
	gr.commit = commit
	gr.hasCommit = true
	gr.QueueDraw()
}

// draw is the Cairo drawing function called by GTK for each frame.
// It renders the lane lines, edges, and commit node for this row.
func (gr *GraphRenderer) draw(cr *cairo.Context, width, height int) {
	if !gr.hasCommit {
		return
	}

	c := gr.commit
	centerY := float64(height) / 2.0

	// --- Draw edges (lines from this commit to its parents) ---
	for _, edge := range c.Edges {
		gr.drawEdge(cr, edge, centerY, height)
	}

	// --- Draw the commit node ---
	nodeX := float64(c.Lane)*laneWidth + laneWidth/2.0
	color := laneColor(c.Lane)

	if c.IsMerge {
		// Merge commits are drawn as diamonds.
		gr.drawDiamond(cr, nodeX, centerY, nodeRadius+1, color)
	} else {
		// Regular commits are circles.
		gr.drawCircle(cr, nodeX, centerY, nodeRadius, color)
	}

	// HEAD commit gets a double-ring.
	if c.IsHead {
		cr.SetSourceRGB(color[0], color[1], color[2])
		cr.SetLineWidth(1.5)
		cr.Arc(nodeX, centerY, nodeRadius+3, 0, 2*3.14159)
		cr.Stroke()
	}
}

// drawEdge draws a connection line from this commit to a parent.
func (gr *GraphRenderer) drawEdge(cr *cairo.Context, edge git.GraphEdge, centerY float64, height int) {
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
func (gr *GraphRenderer) drawCircle(cr *cairo.Context, x, y, radius float64, color [3]float64) {
	cr.SetSourceRGB(color[0], color[1], color[2])
	cr.Arc(x, y, radius, 0, 2*3.14159)
	cr.Fill()
}

// drawDiamond draws a filled diamond (rotated square) at the given position.
// Used for merge commits to visually distinguish them from regular commits.
func (gr *GraphRenderer) drawDiamond(cr *cairo.Context, x, y, size float64, color [3]float64) {
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
