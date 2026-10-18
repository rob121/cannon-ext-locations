package main

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rob121/cannon/extension"
	"gorm.io/gorm"
)

//go:embed help/*.md
var helpFiles embed.FS

//go:embed all:templates
var templateFiles embed.FS

const (
	extensionName        = "cannon-ext-locations"
	extensionTitle       = "Locations"
	extensionDescription = "Manage named map locations with addresses and rich content, rendered with Leaflet on all-locations and single-location pages."
	extensionVersion     = "0.2.1"
	updateURLBase        = "https://github.com/rob121/cannon-ext-locations/releases/download"
	defaultAdminMenu     = "Locations"
	pageSize             = 50
)

var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

type locationsApp struct {
	info      extension.Info
	templates *extension.Templates
	geocode   geocoder

	schemaOnce sync.Once
	schemaErr  error
}

type location struct {
	ID        uint      `gorm:"primaryKey;column:location_id"`
	Name      string    `gorm:"column:name;size:220;not null"`
	Slug      string    `gorm:"column:slug;size:220;not null;index"`
	Address   string    `gorm:"column:address;size:512;not null"`
	Latitude  float64   `gorm:"column:latitude;not null;default:0"`
	Longitude float64   `gorm:"column:longitude;not null;default:0"`
	Content   string    `gorm:"column:content;type:text"`
	SortOrder int       `gorm:"column:sort_order;not null;default:0;index"`
	Status    string    `gorm:"column:status;size:16;not null;default:active;index"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

type locationView struct {
	location
	ContentHTML template.HTML
	URL         string
	HasCoords   bool
}

func main() {
	if err := newServer().Run(); err != nil {
		log.Fatal(err)
	}
}

func newServer() *extension.Server {
	info := extension.Info{
		Name:          extensionName,
		Title:         extensionTitle,
		Description:   extensionDescription,
		Version:       extensionVersion,
		UpdateURLBase: updateURLBase,
		AdminMenuName: defaultAdminMenu,
	}
	app := &locationsApp{
		info:      info,
		templates: extension.NewTemplates(templateFiles, "templates"),
		geocode:   nominatimGeocoder{},
	}

	s := extension.New(info)
	s.EmbedHelp(helpFiles, "help")
	s.EmbedTemplates(templateFiles, "templates")
	s.RegisterPermissions([]extension.PermissionDef{
		{ID: "manage", DisplayName: "Manage Locations", Description: "Create and edit map locations."},
	})
	s.OnInstall(app.install)
	s.OnPageList(app.pageDefinitions)
	s.RegisterPage(extension.PageDefinition{ID: "locations-index", Title: "Locations Map"}, app.indexPage)
	s.RegisterPage(extension.PageDefinition{ID: "locations-detail", Title: "Location Detail"}, app.detailPage)
	s.OnBlockList(app.blockDefinitions)
	s.RegisterBlock(extension.BlockDefinition{
		ID:     "location-map",
		Title:  "Location Map",
		Spaces: []string{"sidebar", "above-main", "below-main", "above-footer"},
	}, app.block)
	s.HandleAdmin("/admin/", app.admin)
	return s
}

func (app *locationsApp) install(req extension.WireRequest) error {
	db, _, err := extension.OpenDB()
	if err != nil {
		return err
	}
	return app.ensureSchema(db)
}

func (app *locationsApp) ensureSchema(db *gorm.DB) error {
	app.schemaOnce.Do(func() {
		app.schemaErr = db.Table(app.locationsTable()).AutoMigrate(&location{})
	})
	return app.schemaErr
}

func (app *locationsApp) locationsTable() string {
	return extension.TableName(app.info.TablePrefix(), "locations")
}

func (app *locationsApp) render(name string, data map[string]any) extension.WireResponse {
	body, err := app.templates.WithFuncs(template.FuncMap{
		"add":       func(a, b int) int { return a + b },
		"sub":       func(a, b int) int { return a - b },
		"mul":       func(a, b int) int { return a * b },
		"pageCount": pageCount,
		"printf":    fmt.Sprintf,
		"safeHTML":  func(v string) template.HTML { return template.HTML(v) },
	}).Execute(name, data)
	if err != nil {
		return extension.Error(http.StatusInternalServerError, err.Error())
	}
	return extension.HTML(http.StatusOK, body)
}

func csrfField(req extension.WireRequest) template.HTML {
	return template.HTML(extension.CSRFHiddenField(req))
}

func parseWireURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return url.Parse("/admin/extension-apps/" + url.PathEscape(extensionName))
	}
	return url.Parse(raw)
}

func adminRoute(path string) (string, []string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == "admin" && parts[i+1] == "extension-apps" {
			base := "/" + strings.Join(parts[:i+3], "/")
			return base, parts[i+3:]
		}
	}
	return "", nil
}

func parseID(value string) (uint, bool) {
	n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil || n == 0 {
		return 0, false
	}
	return uint(n), true
}

func parsePage(u *url.URL) int {
	page, err := strconv.Atoi(u.Query().Get("page"))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func pageCount(total int64, size int) int {
	if total <= 0 || size <= 0 {
		return 1
	}
	return int((total + int64(size) - 1) / int64(size))
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugRE.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func pageString(req extension.WireRequest, key, fallback string) string {
	if v := strings.TrimSpace(extension.PageDataString(req, key)); v != "" {
		return v
	}
	return fallback
}

func requestPathSlug(req extension.WireRequest) string {
	u, err := url.Parse(req.URL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return slugify(parts[len(parts)-1])
}

func joinURL(base, slug string) string {
	base = strings.TrimSpace(base)
	slug = strings.Trim(slug, "/")
	if base == "" {
		return "/" + slug
	}
	return strings.TrimRight(base, "/") + "/" + slug
}

func hasCoords(lat, lng float64) bool {
	return !(lat == 0 && lng == 0)
}
