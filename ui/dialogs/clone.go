// Package dialogs provides AdwDialog-based dialogs for Git operations.
//
// clone.go implements the "Clone Repository" dialog. Per GNOME HIG,
// this is an AdwDialog (not a standalone window) that slides in from
// the bottom or center.
//
// The dialog contains:
//   - URL entry field for the repository URL.
//   - Destination path field with a folder picker button.
//   - A "Clone" button that starts the clone operation.
//   - A progress spinner shown during cloning.
//   - Inline error display if the clone fails.
package dialogs

import (
	"context"
	"log/slog"

	"github.com/MedaiP90/GiTK/git"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// OnCloneComplete is called when a clone operation completes successfully.
// The callback receives the newly cloned repository.
type OnCloneComplete func(repo *git.Repository)

// CloneDialog is the "Clone Repository" dialog.
type CloneDialog struct {
	// dialog is the underlying AdwDialog.
	dialog *adw.Dialog

	// parent is the parent window for positioning.
	parent *adw.ApplicationWindow

	// urlEntry is the text field for the repository URL.
	urlEntry *adw.EntryRow

	// destEntry is the text field for the destination path.
	destEntry *adw.EntryRow

	// cloneBtn is the "Clone" action button.
	cloneBtn *gtk.Button

	// spinner is shown during the clone operation.
	spinner *gtk.Spinner

	// errorBanner shows inline error messages.
	errorBanner *adw.Banner

	// onComplete is called when cloning succeeds.
	onComplete OnCloneComplete
}

// ShowCloneDialog creates and presents the clone dialog.
//
// Parameters:
//   - parent: the parent window.
//   - onComplete: callback when clone succeeds.
func ShowCloneDialog(parent *adw.ApplicationWindow, onComplete OnCloneComplete) {
	d := &CloneDialog{
		parent:     parent,
		onComplete: onComplete,
	}

	d.build()
	d.dialog.Present(parent)
}

// build constructs all the dialog widgets.
func (d *CloneDialog) build() {
	// --- URL entry ---
	d.urlEntry = adw.NewEntryRow()
	d.urlEntry.SetTitle("Repository URL")
	d.urlEntry.SetInputPurpose(gtk.InputPurposeURL)

	// --- Destination entry with folder picker ---
	d.destEntry = adw.NewEntryRow()
	d.destEntry.SetTitle("Destination Path")

	// Folder picker button as a suffix on the dest entry.
	browseBtn := gtk.NewButtonFromIconName("folder-open-symbolic")
	browseBtn.SetTooltipText("Choose folder")
	browseBtn.SetVAlign(gtk.AlignCenter)
	browseBtn.ConnectClicked(func() {
		d.pickDestination()
	})
	d.destEntry.AddSuffix(browseBtn)

	// --- Error banner (hidden by default) ---
	d.errorBanner = adw.NewBanner("")
	d.errorBanner.SetRevealed(false)

	// --- Form layout ---
	formGroup := adw.NewPreferencesGroup()
	formGroup.SetTitle("Clone Repository")
	formGroup.SetDescription("Enter the URL of the Git repository to clone.")
	formGroup.Add(d.urlEntry)
	formGroup.Add(d.destEntry)

	// --- Spinner (hidden until cloning starts) ---
	d.spinner = gtk.NewSpinner()
	d.spinner.SetVisible(false)

	// --- Buttons ---
	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		d.dialog.Close()
	})

	d.cloneBtn = gtk.NewButtonWithLabel("Clone")
	d.cloneBtn.AddCSSClass("suggested-action")
	d.cloneBtn.ConnectClicked(func() {
		d.onClone()
	})

	// Button box.
	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	btnBox.SetHAlign(gtk.AlignEnd)
	btnBox.SetMarginTop(18)
	btnBox.Append(d.spinner)
	btnBox.Append(cancelBtn)
	btnBox.Append(d.cloneBtn)

	// --- Main layout ---
	content := gtk.NewBox(gtk.OrientationVertical, 0)
	content.SetMarginTop(24)
	content.SetMarginBottom(24)
	content.SetMarginStart(24)
	content.SetMarginEnd(24)
	content.Append(d.errorBanner)
	content.Append(formGroup)
	content.Append(btnBox)

	// --- Create the AdwDialog ---
	d.dialog = adw.NewDialog()
	d.dialog.SetTitle("Clone Repository")
	d.dialog.SetContentWidth(450)
	d.dialog.SetContentHeight(300)
	d.dialog.SetChild(content)
}

// pickDestination opens a folder chooser for the clone destination.
func (d *CloneDialog) pickDestination() {
	fileDialog := gtk.NewFileDialog()
	fileDialog.SetTitle("Choose Clone Destination")

	fileDialog.SelectFolder(context.Background(), &d.parent.Window, func(result gio.AsyncResulter) {
		file, err := fileDialog.SelectFolderFinish(result)
		if err != nil {
			// User cancelled — not an error.
			return
		}
		d.destEntry.SetText(file.Path())
	})
}

// onClone starts the clone operation in a background goroutine.
func (d *CloneDialog) onClone() {
	url := d.urlEntry.Text()
	dest := d.destEntry.Text()

	// Validate inputs.
	if url == "" {
		d.showError("Please enter a repository URL.")
		return
	}
	if dest == "" {
		d.showError("Please choose a destination path.")
		return
	}

	// Show progress.
	d.cloneBtn.SetSensitive(false)
	d.spinner.SetVisible(true)
	d.spinner.Start()
	d.errorBanner.SetRevealed(false)

	slog.Info("starting clone", "url", url, "dest", dest)

	// Run the clone in a background goroutine.
	go func() {
		repo, err := git.CloneRepository(url, dest, nil)

		// All GTK updates must happen on the main thread.
		// glib.IdleAdd schedules a function to run on the next iteration
		// of the GTK main loop.
		glib.IdleAdd(func() {
			d.spinner.Stop()
			d.spinner.SetVisible(false)
			d.cloneBtn.SetSensitive(true)

			if err != nil {
				slog.Warn("clone failed", "url", url, "error", err)
				d.showError("Clone failed: " + err.Error())
				return
			}

			slog.Info("clone completed", "url", url, "dest", dest)

			// Close the dialog and notify the caller.
			d.dialog.Close()
			if d.onComplete != nil {
				d.onComplete(repo)
			}
		})
	}()
}

// showError displays an inline error message in the dialog.
func (d *CloneDialog) showError(message string) {
	d.errorBanner.SetTitle(message)
	d.errorBanner.SetRevealed(true)
}
