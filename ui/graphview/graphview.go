// Package graphview implements the full visual DAG (Directed Acyclic Graph)
// view for GiTK.
//
// This view draws the entire commit graph on a large GtkDrawingArea inside
// a GtkScrolledWindow. The user can scroll, zoom (Ctrl+scroll), and click
// on commits or ref pills to select them.
//
// Widget hierarchy:
//
//	AdwToolbarView
//	  ├─ [top] AdwHeaderBar (with zoom controls)
//	  └─ [content] GtkScrolledWindow
//	       └─ GtkDrawingArea (Cairo-drawn graph)
package graphview

import (
	"fmt"
	"log/slog"
	"math"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// laneColors is the GNOME palette used for coloring graph lanes.
// Each lane gets a distinct color to make it easy to follow branches.
var laneColors = [][3]float64{
	{0.16, 0.63, 0.41}, // green
	{0.15, 0.47, 0.82}, // blue
	{0.87, 0.11, 0.14}, // red
	{0.56, 0.25, 0.68}, // purple
	{0.90, 0.62, 0.0},  // orange
	{0.10, 0.74, 0.61}, // teal
	{0.83, 0.18, 0.60}, // magenta
	{0.47, 0.36, 0.27}, // brown
}

// OnCommitSelected is called when the user clicks a commit in the graph.
type OnCommitSelected func(commit git.GraphCommit)

// GraphView is the visual DAG view widget.
type GraphView struct {
	// Root is the top-level widget to embed in the content stack.
	Root *adw.ToolbarView

	// drawArea is the Cairo drawing area.
	drawArea *gtk.DrawingArea

	// scrolled is the scroll container.
	scrolled *gtk.ScrolledWindow

	// layout controls coordinate conversion.
	layout Layout

	// nodes is the list of graph nodes with pixel positions.
	nodes []GraphNode

	// graphCommits is the raw graph data.
	graphCommits []git.GraphCommit

	// selectedRow is the index of the currently selected commit (-1 if none).
	selectedRow int

	// onSelected is the callback for commit selection.
	onSelected OnCommitSelected

	// maxLane tracks the widest lane used in the current graph.
	maxLane int
}

// New creates a new GraphView.
//
// Parameters:
//   - onSelected: callback when the user clicks a commit.
func New(onSelected OnCommitSelected) *GraphView {
	gv := &GraphView{
		layout:      DefaultLayout(),
		selectedRow: -1,
		onSelected:  onSelected,
	}

	gv.build()
	return gv
}

// build constructs the graph view widgets.
func (gv *GraphView) build() {
	// --- Header bar with zoom controls ---
	header := adw.NewHeaderBar()
	header.SetShowBackButton(true)

	// Zoom in button.
	zoomInBtn := gtk.NewButtonFromIconName("zoom-in-symbolic")
	zoomInBtn.SetTooltipText("Zoom In")
	zoomInBtn.ConnectClicked(func() { gv.zoom(4) })
	header.PackEnd(zoomInBtn)

	// Zoom out button.
	zoomOutBtn := gtk.NewButtonFromIconName("zoom-out-symbolic")
	zoomOutBtn.SetTooltipText("Zoom Out")
	zoomOutBtn.ConnectClicked(func() { gv.zoom(-4) })
	header.PackEnd(zoomOutBtn)

	// Zoom reset button.
	resetBtn := gtk.NewButtonFromIconName("zoom-original-symbolic")
	resetBtn.SetTooltipText("Reset Zoom")
	resetBtn.ConnectClicked(func() {
		gv.layout.RowHeight = DefaultRowHeight
		gv.rebuildNodes()
		gv.drawArea.QueueDraw()
	})
	header.PackEnd(resetBtn)

	// --- Drawing area ---
	gv.drawArea = gtk.NewDrawingArea()
	gv.drawArea.SetVExpand(true)
	gv.drawArea.SetHExpand(true)

	// Connect the draw function.
	gv.drawArea.SetDrawFunc(func(area *gtk.DrawingArea, cr *cairo.Context, width, height int) {
		gv.draw(cr, width, height)
	})

	// Handle clicks.
	clickGesture := gtk.NewGestureClick()
	clickGesture.SetButton(1) // Left button.
	clickGesture.ConnectReleased(func(nPress int, x, y float64) {
		gv.onClick(x, y)
	})
	gv.drawArea.AddController(clickGesture)

	// --- Scroll container ---
	gv.scrolled = gtk.NewScrolledWindow()
	gv.scrolled.SetChild(gv.drawArea)
	gv.scrolled.SetPolicy(gtk.PolicyAutomatic, gtk.PolicyAutomatic)

	// --- Assemble ---
	gv.Root = adw.NewToolbarView()
	gv.Root.AddTopBar(header)
	gv.Root.SetContent(gv.scrolled)
}

// SetGraph loads graph data for display.
func (gv *GraphView) SetGraph(commits []git.GraphCommit) {
	gv.graphCommits = commits
	gv.selectedRow = -1
	gv.rebuildNodes()
	gv.drawArea.QueueDraw()
}

// LoadFromRepo loads the graph from a repository.
func (gv *GraphView) LoadFromRepo(repo *git.Repository) {
	go func() {
		commits, err := git.BuildGraph(repo, git.DefaultGraphOptions())
		glib.IdleAdd(func() {
			if err != nil {
				slog.Warn("failed to build graph", "error", err)
				return
			}
			gv.SetGraph(commits)
		})
	}()
}

// rebuildNodes recomputes pixel positions for all graph commits.
func (gv *GraphView) rebuildNodes() {
	gv.maxLane = 0
	gv.nodes = make([]GraphNode, len(gv.graphCommits))

	for i, gc := range gv.graphCommits {
		gv.nodes[i] = NodeFromCommit(gc, i, gv.layout.LaneWidth, gv.layout.RowHeight)
		if gc.Lane > gv.maxLane {
			gv.maxLane = gc.Lane
		}
	}

	gv.layout.UpdateTextOffset(gv.maxLane)

	// Update drawing area size.
	totalH := int(gv.layout.TotalHeight(len(gv.nodes)))
	totalW := int(gv.layout.TotalWidth(gv.maxLane, 800))
	gv.drawArea.SetSizeRequest(totalW, totalH)
}

// zoom adjusts the row height by delta pixels.
func (gv *GraphView) zoom(delta float64) {
	gv.layout.RowHeight = ClampRowHeight(gv.layout.RowHeight + delta)
	gv.rebuildNodes()
	gv.drawArea.QueueDraw()
}

// onClick handles a mouse click on the drawing area.
func (gv *GraphView) onClick(x, y float64) {
	if len(gv.nodes) == 0 {
		return
	}

	result := gv.layout.HitTest(x, y, gv.nodes)
	if result.Row >= 0 && result.Row < len(gv.nodes) {
		// Deselect old.
		if gv.selectedRow >= 0 && gv.selectedRow < len(gv.nodes) {
			gv.nodes[gv.selectedRow].Selected = false
		}

		gv.selectedRow = result.Row
		gv.nodes[result.Row].Selected = true
		gv.drawArea.QueueDraw()

		if gv.onSelected != nil {
			gv.onSelected(gv.nodes[result.Row].Commit)
		}
	}
}

// draw renders the entire graph using Cairo.
func (gv *GraphView) draw(cr *cairo.Context, width, height int) {
	if len(gv.nodes) == 0 {
		// Draw empty state text.
		cr.SetSourceRGB(0.5, 0.5, 0.5)
		cr.MoveTo(float64(width)/2-60, float64(height)/2)
		cr.ShowText("No graph data")
		return
	}

	// Draw edges first (behind nodes).
	for _, node := range gv.nodes {
		for _, edge := range node.Commit.Edges {
			gv.drawEdge(cr, node, edge)
		}
	}

	// Draw commit nodes.
	for _, node := range gv.nodes {
		gv.drawNode(cr, node)
	}

	// Draw commit text and ref pills.
	for i, node := range gv.nodes {
		gv.drawText(cr, &gv.nodes[i], node)
	}
}

// drawEdge draws a single edge from a commit to its parent.
func (gv *GraphView) drawEdge(cr *cairo.Context, node GraphNode, edge git.GraphEdge) {
	fromX := gv.layout.NodeX(edge.FromLane)
	fromY := node.Y
	toX := gv.layout.NodeX(edge.ToLane)
	toY := node.Y + gv.layout.RowHeight

	// Set color based on the source lane.
	col := laneColors[edge.FromLane%len(laneColors)]
	cr.SetSourceRGB(col[0], col[1], col[2])
	cr.SetLineWidth(2)

	if edge.Style == git.EdgeDashed {
		cr.SetDash([]float64{4, 4}, 0)
	}

	if fromX == toX {
		// Straight vertical line.
		cr.MoveTo(fromX, fromY)
		cr.LineTo(toX, toY)
	} else {
		// Bezier curve for lane changes.
		midY := (fromY + toY) / 2
		cr.MoveTo(fromX, fromY)
		cr.CurveTo(fromX, midY, toX, midY, toX, toY)
	}

	cr.Stroke()
	cr.SetDash(nil, 0)
}

// drawNode draws a commit node (circle or diamond for merge).
func (gv *GraphView) drawNode(cr *cairo.Context, node GraphNode) {
	col := laneColors[node.Commit.Lane%len(laneColors)]
	radius := 5.0

	if node.Selected {
		// Draw selection highlight ring.
		cr.SetSourceRGB(col[0], col[1], col[2])
		cr.Arc(node.X, node.Y, radius+3, 0, 2*math.Pi)
		cr.SetLineWidth(2)
		cr.Stroke()
	}

	if node.Commit.IsMerge {
		// Diamond shape for merge commits.
		cr.SetSourceRGB(col[0], col[1], col[2])
		cr.MoveTo(node.X, node.Y-radius)
		cr.LineTo(node.X+radius, node.Y)
		cr.LineTo(node.X, node.Y+radius)
		cr.LineTo(node.X-radius, node.Y)
		cr.ClosePath()
		cr.Fill()
	} else {
		// Circle for regular commits.
		cr.SetSourceRGB(col[0], col[1], col[2])
		cr.Arc(node.X, node.Y, radius, 0, 2*math.Pi)
		cr.Fill()
	}

	if node.Commit.IsHead {
		// Double-ring for HEAD.
		cr.SetSourceRGB(1, 1, 1)
		cr.Arc(node.X, node.Y, radius-2, 0, 2*math.Pi)
		cr.Fill()
		cr.SetSourceRGB(col[0], col[1], col[2])
		cr.Arc(node.X, node.Y, radius-2, 0, 2*math.Pi)
		cr.SetLineWidth(1.5)
		cr.Stroke()
	}
}

// drawText draws the commit subject text and ref pills next to the node.
func (gv *GraphView) drawText(cr *cairo.Context, nodePtr *GraphNode, node GraphNode) {
	textX := gv.layout.TextOffsetX + 8
	textY := node.Y + 4

	// Draw ref pills first.
	pillX := textX
	for i, ref := range node.Commit.Refs {
		pillW := float64(len(ref.Name)*7) + gv.layout.RefPillPadding*2
		pillH := gv.layout.RefPillHeight
		pillY := node.Y - pillH/2

		// Pill background color based on ref kind.
		var r, g, b float64
		switch ref.Kind {
		case git.RefLocalBranch:
			r, g, b = 0.15, 0.47, 0.82
		case git.RefRemoteBranch:
			r, g, b = 0.56, 0.25, 0.68
		case git.RefTag:
			r, g, b = 0.90, 0.62, 0.0
		case git.RefHEAD:
			r, g, b = 0.16, 0.63, 0.41
		}

		// Draw rounded rectangle.
		drawRoundedRect(cr, pillX, pillY, pillW, pillH, 4)
		cr.SetSourceRGB(r, g, b)
		cr.Fill()

		// Draw pill text.
		cr.SetSourceRGB(1, 1, 1)
		cr.MoveTo(pillX+gv.layout.RefPillPadding, pillY+pillH-4)
		cr.ShowText(ref.Name)

		// Store pill rect for hit testing.
		if i < len(nodePtr.RefPills) {
			nodePtr.RefPills[i] = RefPillRect{
				Ref: ref, X: pillX, Y: pillY, Width: pillW, Height: pillH,
			}
		} else {
			nodePtr.RefPills = append(nodePtr.RefPills, RefPillRect{
				Ref: ref, X: pillX, Y: pillY, Width: pillW, Height: pillH,
			})
		}

		pillX += pillW + 4
	}

	// Draw commit subject.
	cr.SetSourceRGB(0.9, 0.9, 0.9)
	cr.MoveTo(pillX+4, textY)

	subject := node.Commit.Subject
	if len(subject) > 80 {
		subject = subject[:77] + "..."
	}
	cr.ShowText(fmt.Sprintf("%s  %s", node.Commit.ShortHash, subject))
}

// drawRoundedRect draws a rounded rectangle path.
func drawRoundedRect(cr *cairo.Context, x, y, w, h, r float64) {
	cr.MoveTo(x+r, y)
	cr.LineTo(x+w-r, y)
	cr.Arc(x+w-r, y+r, r, -math.Pi/2, 0)
	cr.LineTo(x+w, y+h-r)
	cr.Arc(x+w-r, y+h-r, r, 0, math.Pi/2)
	cr.LineTo(x+r, y+h)
	cr.Arc(x+r, y+h-r, r, math.Pi/2, math.Pi)
	cr.LineTo(x, y+r)
	cr.Arc(x+r, y+r, r, math.Pi, 3*math.Pi/2)
	cr.ClosePath()
}
