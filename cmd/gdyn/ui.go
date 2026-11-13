package main

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type formTheme struct {
	name                                          string
	background, panel, field, text, accent, muted string
}

func configureUI(path string) error {
	cfg, err := loadConfig(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	app := tview.NewApplication()
	form := tview.NewForm()
	pages := tview.NewPages()
	themes := []formTheme{
		{"Slate", "#0b1220", "#111c2e", "#273449", "#e2e8f0", "#60a5fa", "#94a3b8"},
		{"Nord", "#242933", "#2e3440", "#3b4252", "#eceff4", "#88c0d0", "#b3bfd1"},
		{"Dracula", "#1e1f29", "#282a36", "#383a4b", "#f8f8f2", "#ff79c6", "#b6abc9"},
		{"Paper", "#e7e2d8", "#faf7f0", "#e9e4db", "#292d35", "#245b85", "#62666d"},
	}
	themeIndex := 0
	var modal *tview.Modal
	form.AddPasswordField("Gandi token", cfg.Gandi.Token, 40, '*', nil)
	form.AddInputField("Domain", cfg.DNS.Domain, 40, nil, nil)
	form.AddInputField("Records", strings.Join(cfg.DNS.Records, ", "), 40, nil, nil)
	form.AddInputField("TTL (seconds)", strconv.Itoa(cfg.DNS.TTL), 40, nil, nil)
	form.AddInputField("API URL", cfg.Gandi.URL, 40, nil, nil)
	form.AddInputField("IP resolver", cfg.IP.ResolverURL, 40, nil, nil)
	showError := func(message string) {
		modal = tview.NewModal().SetText(tview.Escape(message)).AddButtons([]string{"OK"})
		themes[themeIndex].styleModal(modal)
		modal.SetDoneFunc(func(_ int, _ string) {
			pages.RemovePage("error")
			modal = nil
			app.SetFocus(form)
		})
		pages.AddPage("error", modal, true, true)
		app.SetFocus(modal)
	}
	form.AddButton("Save", func() {
		cfg.Gandi.Token = form.GetFormItem(0).(*tview.InputField).GetText()
		cfg.DNS.Domain = form.GetFormItem(1).(*tview.InputField).GetText()
		cfg.DNS.Records = strings.Split(form.GetFormItem(2).(*tview.InputField).GetText(), ",")
		ttl, err := strconv.Atoi(strings.TrimSpace(form.GetFormItem(3).(*tview.InputField).GetText()))
		if err != nil {
			showError("TTL must be an integer between 300 and 2592000 seconds.")
			return
		}
		cfg.DNS.TTL = ttl
		cfg.Gandi.URL = form.GetFormItem(4).(*tview.InputField).GetText()
		cfg.IP.ResolverURL = form.GetFormItem(5).(*tview.InputField).GetText()
		if err := saveConfig(path, cfg); err != nil {
			showError(err.Error())
			return
		}
		app.Stop()
	})
	form.AddButton("Cancel", app.Stop)
	form.SetCancelFunc(app.Stop)
	form.SetButtonsAlign(tview.AlignCenter)
	form.SetBorder(true)
	help := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetDynamicColors(true).
		SetText("tab[::b].[::-] next field    enter[::b].[::-] select    f2[::b].[::-] theme    esc[::b].[::-] cancel")
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 0, 1, true).
		AddItem(help, 1, 0, false)
	applyTheme := func() {
		theme := themes[themeIndex]
		pages.SetBackgroundColor(tcell.GetColor(theme.background))
		layout.SetBackgroundColor(tcell.GetColor(theme.background))
		help.SetBackgroundColor(tcell.GetColor(theme.background))
		help.SetTextColor(tcell.GetColor(theme.muted))
		form.SetBackgroundColor(tcell.GetColor(theme.panel))
		form.SetBorderColor(tcell.GetColor(theme.accent))
		form.SetTitleColor(tcell.GetColor(theme.accent))
		form.SetTitle(" gdyn configuration · " + strings.ToLower(theme.name) + " ")
		form.SetLabelColor(tcell.GetColor(theme.text))
		form.SetFieldBackgroundColor(tcell.GetColor(theme.field))
		form.SetFieldTextColor(tcell.GetColor(theme.text))
		form.SetButtonStyle(tcell.StyleDefault.Background(tcell.GetColor(theme.field)).Foreground(tcell.GetColor(theme.text)))
		form.SetButtonActivatedStyle(tcell.StyleDefault.Background(tcell.GetColor(theme.accent)).Foreground(tcell.GetColor(theme.panel)))
		if modal != nil {
			theme.styleModal(modal)
		}
	}
	applyTheme()
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyF2 {
			themeIndex = (themeIndex + 1) % len(themes)
			applyTheme()
			return nil
		}
		return event
	})
	app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		width, height := screen.Size()
		cardWidth, cardHeight := min(72, width), min(19, height)
		layout.SetRect((width-cardWidth)/2, (height-cardHeight)/2, cardWidth, cardHeight)
		return false
	})
	pages.AddPage("form", layout, false, true)
	return app.SetRoot(pages, true).EnablePaste(true).Run()
}

func (theme formTheme) styleModal(modal *tview.Modal) {
	modal.SetBackgroundColor(tcell.GetColor(theme.panel))
	modal.SetTextColor(tcell.GetColor(theme.text))
	modal.SetBorderColor(tcell.GetColor(theme.accent))
	modal.SetButtonStyle(tcell.StyleDefault.Background(tcell.GetColor(theme.field)).Foreground(tcell.GetColor(theme.text)))
	modal.SetButtonActivatedStyle(tcell.StyleDefault.Background(tcell.GetColor(theme.accent)).Foreground(tcell.GetColor(theme.panel)))
}
