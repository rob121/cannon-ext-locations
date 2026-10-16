# Locations

Manage named map locations and show them with [Leaflet](https://leafletjs.com/) on public Cannon pages.

## What it provides

- Admin CRUD for locations (name, slug, address, optional lat/lng, WYSIWYG content, sort, status).
- Automatic geocoding via OpenStreetMap Nominatim when coordinates are left blank.
- Page types:
  - **Locations Map** — all active locations on one map plus a linked list.
  - **Location Detail** — one location map and rich content (by selected location or URL slug).
- Block type **Location Map** for embedding all or a single location in template spaces.

## Setup

1. Activate **Locations** under System → Extensions.
2. Create locations in the Locations admin menu.
3. Add Extension routes:
   - `/locations` → page `locations-index` (set **Detail Base URL** to `/locations`).
   - `/locations/*` → page `locations-detail` (optional Index URL `/locations`).

## Tables

Install creates `locations_locations` (prefixed by the extension table prefix).
