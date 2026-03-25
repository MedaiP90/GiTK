// Package graphview — node.go defines the GraphNode UI model.
//
// A GraphNode wraps a git.GraphCommit with additional UI-specific data
// that is needed for rendering and interaction in the visual graph view.
// The git.GraphCommit contains layout data (lane, edges, refs) computed
// by the graph algorithm, while GraphNode adds pixel positions, visual
// state (selected, hovered), and bounding boxes for hit detection.
//
// Why a separate type?
// The git package has ZERO GTK imports — it only computes abstract
// layout (lane numbers, edge connections). This package converts those
// abstract positions into concrete pixel coordinates for Cairo drawing
// and mouse click detection.
package graphview

import (
	"github.com/MedaiP90/GiTK/git"
)

// GraphNode is a UI-enriched wrapper around git.GraphCommit.
// It stores everything the drawing and hit-testing code needs
// for a single commit in the visual graph.
type GraphNode struct {
	// Commit is the underlying graph commit data from the git package.
	// It contains the hash, subject, author, lane, edges, refs, etc.
	Commit git.GraphCommit

	// Row is the index of this commit in the displayed list (0 = top).
	// This is used to convert between list position and pixel Y coordinate.
	Row int

	// X is the horizontal pixel coordinate of the commit node center.
	// Computed from the lane index: X = lane * laneWidth + laneWidth/2.
	X float64

	// Y is the vertical pixel coordinate of the commit node center.
	// Computed from the row index: Y = row * rowHeight + rowHeight/2.
	Y float64

	// Selected is true when this commit is the currently selected one.
	// A selected node is drawn with a highlight ring around it.
	Selected bool

	// Hovered is true when the mouse cursor is over this node.
	// A hovered node is drawn slightly larger for visual feedback.
	Hovered bool

	// RefPills holds the bounding boxes for each ref label pill drawn
	// next to this commit. This allows the context menu code to know
	// which specific ref was right-clicked.
	RefPills []RefPillRect
}

// RefPillRect stores the bounding rectangle of a rendered ref label pill.
// This is used for hit-testing: when the user clicks on or near a ref
// pill, we can identify which ref they clicked on.
type RefPillRect struct {
	// Ref is the git ref (branch/tag) this pill represents.
	Ref git.GraphRef

	// X is the left edge of the pill rectangle in pixels.
	X float64

	// Y is the top edge of the pill rectangle in pixels.
	Y float64

	// Width is the pill rectangle width in pixels.
	Width float64

	// Height is the pill rectangle height in pixels.
	Height float64
}

// ContainsPoint returns true if the given pixel coordinate falls inside
// this ref pill rectangle. Used for hit-testing on mouse click.
func (r RefPillRect) ContainsPoint(px, py float64) bool {
	return px >= r.X && px <= r.X+r.Width &&
		py >= r.Y && py <= r.Y+r.Height
}

// NodeFromCommit creates a GraphNode from a git.GraphCommit and a row
// index. It computes the pixel coordinates using the provided laneW
// (lane width) and rowH (row height) values.
//
// Parameters:
//   - gc: the graph commit data from git.BuildGraph().
//   - row: the index of this commit in the visible list (0 = first).
//   - laneW: horizontal spacing between lanes in pixels.
//   - rowH: vertical spacing between rows in pixels.
func NodeFromCommit(gc git.GraphCommit, row int, laneW, rowH float64) GraphNode {
	return GraphNode{
		Commit: gc,
		Row:    row,
		X:      float64(gc.Lane)*laneW + laneW/2.0,
		Y:      float64(row)*rowH + rowH/2.0,
	}
}
