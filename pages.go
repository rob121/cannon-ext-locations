package main

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/rob121/cannon/extension"
	"gorm.io/gorm"
)

func (app *locationsApp) pageDefinitions() ([]extension.PageDefinition, error) {
	options := app.activeLocationOptions()
	common := []extension.PageField{
		{Name: "title", Label: "Page Title", Type: "text", Help: "Optional heading override."},
		{Name: "introduction", Label: "Introduction", Type: "textarea", Help: "Optional introductory copy above the map."},
		{Name: "detail_base_url", Label: "Detail Base URL", Type: "text", Help: "Public path prefix for location detail links (for example /locations)."},
	}
	return []extension.PageDefinition{
		{
			ID:     "locations-index",
			Title:  "Locations Map",
			Fields: common,
		},
		{
			ID:    "locations-detail",
			Title: "Location Detail",
			Fields: append([]extension.PageField{
				{Name: "location_id", Label: "Location", Type: "select", Help: "Leave empty to infer the location slug from the URL.", Options: options},
				{Name: "index_url", Label: "Index URL", Type: "text", Help: "Optional link back to the all-locations page."},
			}, common...),
		},
	}, nil
}

func (app *locationsApp) blockDefinitions() ([]extension.BlockDefinition, error) {
	return []extension.BlockDefinition{
		{
			ID:     "location-map",
			Title:  "Location Map",
			Spaces: []string{"sidebar", "above-main", "below-main", "above-footer"},
			Fields: []extension.BlockField{
				{
					Name:  "mode",
					Label: "Mode",
					Type:  "select",
					Help:  "Show every active location or a single location.",
					Options: []extension.BlockFieldOption{
						{Value: "all", Label: "All locations"},
						{Value: "single", Label: "Single location"},
					},
				},
				{
					Name:    "location_id",
					Label:   "Location",
					Type:    "select",
					Help:    "Used when mode is Single location.",
					Options: app.activeLocationOptions(),
				},
				{
					Name:  "detail_base_url",
					Label: "Detail Base URL",
					Type:  "text",
					Help:  "Optional path prefix for marker popup links.",
				},
				{
					Name:  "height",
					Label: "Map Height (px)",
					Type:  "text",
					Help:  "Defaults to 360.",
				},
			},
		},
	}, nil
}

func (app *locationsApp) indexPage(item string, req extension.WireRequest) extension.WireResponse {
	views, err := app.activeLocationViews(req)
	if err != nil {
		return extension.Error(http.StatusInternalServerError, err.Error())
	}
	return app.render("locations/index.html", map[string]any{
		"Title":        pageString(req, "title", "Locations"),
		"Introduction": extension.PageDataString(req, "introduction"),
		"Locations":    views,
		"MapJSON":      mapPointsJSON(views),
		"PageItem":     item,
	})
}

func (app *locationsApp) detailPage(item string, req extension.WireRequest) extension.WireResponse {
	loc, found, err := app.findActiveLocation(req)
	if err != nil {
		return extension.Error(http.StatusInternalServerError, err.Error())
	}
	if !found {
		return extension.Error(http.StatusNotFound, "location not found")
	}
	view := app.locationView(loc, extension.PageDataString(req, "detail_base_url"))
	return app.render("locations/detail.html", map[string]any{
		"Title":        pageString(req, "title", loc.Name),
		"Introduction": extension.PageDataString(req, "introduction"),
		"Location":     view,
		"MapJSON":      mapPointsJSON([]locationView{view}),
		"IndexURL":     strings.TrimSpace(extension.PageDataString(req, "index_url")),
		"PageItem":     item,
	})
}

func (app *locationsApp) block(item string, req extension.WireRequest) extension.WireResponse {
	mode := strings.ToLower(strings.TrimSpace(extension.BlockDataString(req, "mode")))
	if mode == "" {
		mode = "all"
	}
	detailBase := extension.BlockDataString(req, "detail_base_url")
	height := strings.TrimSpace(extension.BlockDataString(req, "height"))
	if height == "" {
		height = "360"
	}

	var views []locationView
	if mode == "single" {
		id, ok := extension.BlockDataInt(req, "location_id")
		if !ok || id <= 0 {
			return extension.HTML(http.StatusOK, "")
		}
		loc, found, err := app.locationByID(uint(id), true)
		if err != nil {
			return extension.Error(http.StatusInternalServerError, err.Error())
		}
		if !found {
			return extension.HTML(http.StatusOK, "")
		}
		views = []locationView{app.locationView(loc, detailBase)}
	} else {
		var err error
		views, err = app.activeLocationViewsWithBase(detailBase)
		if err != nil {
			return extension.Error(http.StatusInternalServerError, err.Error())
		}
	}
	if len(views) == 0 {
		return extension.HTML(http.StatusOK, "")
	}
	return app.render("locations/block.html", map[string]any{
		"Locations": views,
		"MapJSON":   mapPointsJSON(views),
		"Height":    height,
		"Space":     extension.BlockSpace(req),
		"BlockItem": item,
	})
}

func (app *locationsApp) activeLocationOptions() []extension.BlockFieldOption {
	db, _, err := extension.OpenDB()
	if err != nil {
		return nil
	}
	var rows []location
	if err := db.Table(app.locationsTable()).Where("status = ?", "active").Order("sort_order asc, name asc, location_id asc").Find(&rows).Error; err != nil {
		return nil
	}
	out := make([]extension.BlockFieldOption, 0, len(rows))
	for _, row := range rows {
		out = append(out, extension.BlockFieldOption{
			Value: strconv.FormatUint(uint64(row.ID), 10),
			Label: row.Name,
		})
	}
	return out
}

func (app *locationsApp) activeLocationViews(req extension.WireRequest) ([]locationView, error) {
	return app.activeLocationViewsWithBase(extension.PageDataString(req, "detail_base_url"))
}

func (app *locationsApp) activeLocationViewsWithBase(detailBase string) ([]locationView, error) {
	db, _, err := extension.OpenDB()
	if err != nil {
		return nil, err
	}
	var rows []location
	if err := db.Table(app.locationsTable()).Where("status = ?", "active").Order("sort_order asc, name asc, location_id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]locationView, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.locationView(row, detailBase))
	}
	return out, nil
}

func (app *locationsApp) findActiveLocation(req extension.WireRequest) (location, bool, error) {
	if id, ok := extension.PageDataInt(req, "location_id"); ok && id > 0 {
		return app.locationByID(uint(id), true)
	}
	slug := requestPathSlug(req)
	if slug == "" {
		return location{}, false, nil
	}
	return app.locationBySlug(slug, true)
}

func (app *locationsApp) locationByID(id uint, activeOnly bool) (location, bool, error) {
	db, _, err := extension.OpenDB()
	if err != nil {
		return location{}, false, err
	}
	q := db.Table(app.locationsTable()).Where("location_id = ?", id)
	if activeOnly {
		q = q.Where("status = ?", "active")
	}
	var row location
	err = q.First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return location{}, false, nil
	}
	if err != nil {
		return location{}, false, err
	}
	return row, true, nil
}

func (app *locationsApp) locationBySlug(slug string, activeOnly bool) (location, bool, error) {
	db, _, err := extension.OpenDB()
	if err != nil {
		return location{}, false, err
	}
	q := db.Table(app.locationsTable()).Where("slug = ?", slug)
	if activeOnly {
		q = q.Where("status = ?", "active")
	}
	var row location
	err = q.First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return location{}, false, nil
	}
	if err != nil {
		return location{}, false, err
	}
	return row, true, nil
}

func (app *locationsApp) locationView(row location, detailBase string) locationView {
	return locationView{
		location:    row,
		ContentHTML: template.HTML(row.Content),
		URL:         joinURL(detailBase, row.Slug),
		HasCoords:   hasCoords(row.Latitude, row.Longitude),
	}
}

func mapPointsJSON(views []locationView) template.JS {
	parts := make([]string, 0, len(views))
	for _, view := range views {
		if !view.HasCoords {
			continue
		}
		parts = append(parts, `{`+
			`"id":`+strconv.FormatUint(uint64(view.ID), 10)+`,`+
			`"name":`+jsonString(view.Name)+`,`+
			`"address":`+jsonString(view.Address)+`,`+
			`"url":`+jsonString(view.URL)+`,`+
			`"lat":`+strconv.FormatFloat(view.Latitude, 'f', -1, 64)+`,`+
			`"lng":`+strconv.FormatFloat(view.Longitude, 'f', -1, 64)+
			`}`)
	}
	return template.JS("[" + strings.Join(parts, ",") + "]")
}

func jsonString(value string) string {
	b, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(b)
}
