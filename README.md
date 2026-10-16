# cannon-ext-locations

Cannon extension for managing map locations and rendering them with [Leaflet](https://leafletjs.com/).

## Features

- Admin UI to create N locations: **name**, **address**, optional **lat/lng**, **WYSIWYG content**
- Geocodes address with Nominatim when coordinates are empty
- Frontend pages: all locations map, single location detail
- Optional block for embedding a map

## Development

```bash
go mod tidy
go build -o bin/cannon-ext-locations .
```

Point Cannon's extension build at this module (replace path already set to `../cannon`).

## Routes

| Page ID | Purpose |
|---------|---------|
| `locations-index` | Map + list of all active locations |
| `locations-detail` | Single location; resolve by `location_id` field or URL slug |

## Permissions

- `cannon-ext-locations.manage` — create/edit/delete locations
