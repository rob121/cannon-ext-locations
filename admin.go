package main

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/rob121/cannon/extension"
	"gorm.io/gorm"
)

func (app *locationsApp) admin(req extension.WireRequest) extension.WireResponse {
	if !extension.UserAuthenticated(req) {
		return extension.Error(http.StatusForbidden, "sign in required")
	}

	db, _, err := extension.OpenDB()
	if err != nil {
		return extension.Error(http.StatusInternalServerError, err.Error())
	}
	if err := db.Table(app.locationsTable()).AutoMigrate(&location{}); err != nil {
		return extension.Error(http.StatusInternalServerError, err.Error())
	}

	adminURL, err := parseWireURL(req.URL)
	if err != nil {
		return extension.Error(http.StatusBadRequest, err.Error())
	}
	basePath, parts := adminRoute(adminURL.Path)
	if basePath == "" {
		basePath = "/admin/extension-apps/" + url.PathEscape(extensionName)
	}
	csrf := csrfField(req)

	switch {
	case len(parts) == 0:
		return app.listLocations(db, adminURL, basePath, "", csrf)
	case len(parts) == 1 && parts[0] == "new":
		if strings.EqualFold(req.Method, http.MethodPost) {
			return app.saveLocation(db, req.Body, adminURL, basePath, 0, csrf)
		}
		return app.renderLocationForm(basePath, location{Status: "active", SortOrder: 0}, true, "", csrf)
	case len(parts) == 1:
		id, ok := parseID(parts[0])
		if !ok {
			return extension.Error(http.StatusNotFound, "location not found")
		}
		if strings.EqualFold(req.Method, http.MethodPost) {
			return app.saveLocation(db, req.Body, adminURL, basePath, id, csrf)
		}
		return app.editLocation(db, basePath, id, "", csrf)
	case len(parts) == 2 && parts[1] == "delete":
		id, ok := parseID(parts[0])
		if !ok {
			return extension.Error(http.StatusNotFound, "location not found")
		}
		if !strings.EqualFold(req.Method, http.MethodPost) {
			return extension.Error(http.StatusMethodNotAllowed, "method not allowed")
		}
		return app.deleteLocation(db, adminURL, basePath, id, csrf)
	default:
		return extension.Error(http.StatusNotFound, "admin route not found")
	}
}

func (app *locationsApp) listLocations(db *gorm.DB, adminURL *url.URL, basePath, notice string, csrf template.HTML) extension.WireResponse {
	page := parsePage(adminURL)
	var total int64
	if err := db.Table(app.locationsTable()).Count(&total).Error; err != nil {
		return extension.Error(http.StatusInternalServerError, err.Error())
	}
	var activeTotal int64
	if err := db.Table(app.locationsTable()).Where("status = ?", "active").Count(&activeTotal).Error; err != nil {
		return extension.Error(http.StatusInternalServerError, err.Error())
	}
	var rows []location
	err := db.Table(app.locationsTable()).
		Order("sort_order asc, name asc, location_id asc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&rows).Error
	if err != nil {
		return extension.Error(http.StatusInternalServerError, err.Error())
	}
	return app.render("locations/admin_list.html", map[string]any{
		"Title":           "Locations",
		"Subtitle":        "Manage map locations with addresses and rich content.",
		"PageActionURL":   basePath + "/new",
		"PageActionLabel": "Add Location",
		"BasePath":        basePath,
		"Rows":            rows,
		"Page":            page,
		"PageSize":        pageSize,
		"Total":           total,
		"ActiveTotal":     activeTotal,
		"Notice":          notice,
		"CSRFField":       csrf,
	})
}

func (app *locationsApp) editLocation(db *gorm.DB, basePath string, id uint, errMsg string, csrf template.HTML) extension.WireResponse {
	var row location
	err := db.Table(app.locationsTable()).Where("location_id = ?", id).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return extension.Error(http.StatusNotFound, "location not found")
	}
	if err != nil {
		return extension.Error(http.StatusInternalServerError, err.Error())
	}
	return app.renderLocationForm(basePath, row, false, errMsg, csrf)
}

func (app *locationsApp) saveLocation(db *gorm.DB, body string, adminURL *url.URL, basePath string, id uint, csrf template.HTML) extension.WireResponse {
	row, errMsg := locationFromBody(body, id)
	if errMsg != "" {
		return app.renderLocationForm(basePath, row, id == 0, errMsg, csrf)
	}
	if id != 0 {
		var existing location
		err := db.Table(app.locationsTable()).Where("location_id = ?", id).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			return extension.Error(http.StatusNotFound, "location not found")
		}
		if err != nil {
			return extension.Error(http.StatusInternalServerError, err.Error())
		}
		row.ID = id
		row.CreatedAt = existing.CreatedAt
		if row.Latitude == 0 && row.Longitude == 0 && existing.Address == row.Address {
			row.Latitude = existing.Latitude
			row.Longitude = existing.Longitude
		}
	}
	if row.Slug == "" {
		row.Slug = slugify(row.Name)
	}
	if row.Slug == "" {
		return app.renderLocationForm(basePath, row, id == 0, "Slug is required.", csrf)
	}
	notice := "Location saved."
	if !hasCoords(row.Latitude, row.Longitude) {
		lat, lng, err := app.geocode.Lookup(row.Address)
		if err != nil {
			notice = "Location saved. Geocoding did not find coordinates for this address — add latitude/longitude manually if you want it on the map."
		} else {
			row.Latitude = lat
			row.Longitude = lng
		}
	}
	if err := app.ensureUniqueSlug(db, &row); err != nil {
		return app.renderLocationForm(basePath, row, id == 0, err.Error(), csrf)
	}
	if err := db.Table(app.locationsTable()).Save(&row).Error; err != nil {
		return app.renderLocationForm(basePath, row, id == 0, err.Error(), csrf)
	}
	return app.listLocations(db, adminURL, basePath, notice, csrf)
}

func (app *locationsApp) deleteLocation(db *gorm.DB, adminURL *url.URL, basePath string, id uint, csrf template.HTML) extension.WireResponse {
	if err := db.Table(app.locationsTable()).Where("location_id = ?", id).Delete(&location{}).Error; err != nil {
		return extension.Error(http.StatusInternalServerError, err.Error())
	}
	return app.listLocations(db, adminURL, basePath, "Location deleted.", csrf)
}

func (app *locationsApp) renderLocationForm(basePath string, row location, isNew bool, errMsg string, csrf template.HTML) extension.WireResponse {
	title := "Edit Location"
	action := fmt.Sprintf("%s/%d", basePath, row.ID)
	if isNew {
		title = "Add Location"
		action = basePath + "/new"
	}
	return app.render("locations/admin_form.html", map[string]any{
		"Title":     title,
		"Subtitle":  "Name, address, optional coordinates, and rich content for the public map pages.",
		"BasePath":  basePath,
		"ActionURL": action,
		"CancelURL": basePath,
		"Row":       row,
		"IsNew":     isNew,
		"Error":     errMsg,
		"CSRFField": csrf,
	})
}

func (app *locationsApp) ensureUniqueSlug(db *gorm.DB, row *location) error {
	base := row.Slug
	for i := 0; i < 50; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}
		var count int64
		q := db.Table(app.locationsTable()).Where("slug = ?", candidate)
		if row.ID != 0 {
			q = q.Where("location_id <> ?", row.ID)
		}
		if err := q.Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			row.Slug = candidate
			return nil
		}
	}
	return fmt.Errorf("could not find a unique slug")
}

func locationFromBody(body string, id uint) (location, string) {
	values, err := url.ParseQuery(body)
	if err != nil {
		return location{}, err.Error()
	}
	row := location{
		ID:        id,
		Name:      strings.TrimSpace(values.Get("name")),
		Slug:      slugify(values.Get("slug")),
		Address:   strings.TrimSpace(values.Get("address")),
		Content:   strings.TrimSpace(values.Get("content")),
		Status:    strings.TrimSpace(values.Get("status")),
		SortOrder: atoiDefault(values.Get("sort_order"), 0),
		Latitude:  atofDefault(values.Get("latitude"), 0),
		Longitude: atofDefault(values.Get("longitude"), 0),
	}
	if row.Status != "inactive" {
		row.Status = "active"
	}
	if row.Name == "" {
		return row, "Name is required."
	}
	if row.Address == "" {
		return row, "Address is required."
	}
	return row, ""
}

func atoiDefault(raw string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return n
}

func atofDefault(raw string, fallback float64) float64 {
	n, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return fallback
	}
	return n
}
