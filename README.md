# Manju-backend
# PROJECT:Operational Intelligence OS for Enterprises

## WHAT I'M BUILDING

I am building an operational intelligence platform — think "Palantir Foundry" 
but for underserved markets (MENA, Asia) and accessible to both enterprises 
and MSMEs. The platform is designed as an "operating system for the physical 
world" where the core kernel is industry-agnostic and domain-specific 
functionality is delivered through a plugin system.

The first vertical I'm targeting is **wharehouse that delivers packages**, starting with 
Gulf region civil defense organizations (UAE, Saudi Arabia, Qatar).

## THE ARCHITECTURE — THREE LAYERS

### Layer 1: The Kernel (Industry-Agnostic)
The kernel knows nothing abouttrucks or factories. It only understands:
- **Objects**: Any real-world entity (a truck, a person, a building, a machine)
- **Properties**: Typed attributes on objects (string, integer, geo_point, enum, etc.)
- **Relationships**: Links between objects (stationed_at, responds_to, supplies)
- **Events**: Things that happen (object.created, object.updated, status.changed)
- **Actions**: Things users or systems can DO to objects (dispatch, schedule, flag)

The kernel provides these services:
- **Ontology Engine**: Define and manage object types, properties, relationships
- **Data Engine**: CRUD + spatial + graph + full-text queries on objects
- **Digestion Engine**: Connect to external databases, discover schemas, sync data, track changes
- **Event Bus**: Publish/subscribe to events across the platform
- **Pipeline Orchestrator**: Scheduled and triggered data transformations
- **Auth/Permissions**: RBAC + ABAC + row-level security
- **AI Gateway**: Natural language queries, report generation, anomaly detection
- **Storage Engine**: Versioned data storage with time-travel capability

### Layer 2: The Shell (Workspace UI Framework)
The shell is the window manager. It provides:
- Navigation, layout engine, theming
- Shared Context (selected objects, time range, spatial filter, active filters)
- Command palette / global search
- Notification center
- Plugin mounting system (plugins render inside shell-managed regions)

Plugins communicate through Shared Context, not directly with each other. 
When a user clicks "Engine 7" on the map, the Shared Context updates, and 
every other plugin (detail panel, timeline, analytics) reacts automatically.

### Layer 3: Plugin Apps (Domain-Specific)
Plugins are self-contained applications that use kernel services via a Plugin SDK. 
They declare what kernel capabilities they need and what they provide to other plugins.

Generic plugins (work for any industry):
- Map View (any object with geo_point)
- Table/Spreadsheet View
- Dashboard Builder
- Graph Explorer (ontology relationships)
- Timeline View
- Kanban Board (any object with status enum)
- Alert Manager
- AI Chat
- Form Builder
- Report Generator

Domain-specific plugins (fire department):
- Coverage Zone Analyzer
- Dispatch Console
- Pre-Incident Plan Viewer
- Response Time Optimizer
- Firefighter Safety Tracker

The SAME generic plugins render differently based on the ontology. The Map plugin 
shows fire trucks for a fire department and shipments for a manufacturer — no code 
changes, just different ontology objects with geo_point properties.

### Industry Templates
An industry template is: Ontology + Default Plugins + Default Workspaces.
- Fire Department template: Stations, Apparatus, Personnel, Incidents, Buildings, etc.
- Manufacturing template: Factories, Assembly Lines, Suppliers, Shipments, Products, etc.
- A customer selects a template, connects their data, and has a working platform in hours.

## TECHNOLOGY STACK

- **Primary Backend Language**: Go/Java (microservices)
- **AI/LLM Service**: Python (FastAPI) — isolated microservice
- **Frontend**: TypeScript (React/Next.js + MapLibre GL + Shadcn/ui)
- **Primary Database**: PostgreSQL 16 with PostGIS + Apache AGE + pgvector (or AWS RDS later)
- **Cache/PubSub**: Redis 7
- **Object Storage**: MinIO (S3-compatible)
- **Routing Engine**: OSRM (self-hosted, for travel time calculations)
- **Infrastructure**: Docker + K3s (POC), Kubernetes (production)
- **LLM**: GPT-4o via API (POC), Llama 3.1 on-prem (production for government clients)

Key design decision: PostgreSQL with extensions (PostGIS, AGE, pgvector) serves as 
relational DB + spatial DB + graph DB + vector DB in one, minimizing infrastructure 
for the POC.

## DATA MODEL — THE KERNEL SCHEMA

The kernel stores everything in these tables:

**Ontology Schema (blueprint):**
- `object_types` — defines classes of objects (Station, Apparatus, Incident)
- `property_definitions` — defines properties per object type with types, constraints, semantics
- `relationship_types` — defines how object types relate (stationed_at, responds_to)

**Instance Data:**
- `objects` — actual instances, properties stored as JSONB, location as PostGIS geometry
- `relationships` — links between object instances
- `events` — append-only event log

**Digestion Engine:**
- `sources` — registered external databases
- `discovered_tables` — schema catalog of source databases
- `discovered_columns` — column-level metadata with semantic type detection
- `discovered_foreign_keys` — relationships between source tables
- `sync_jobs` — execution history of sync operations
- `ingested_rows` — actual data copied from sources, stored as JSONB with row hashing
- `changes` — change detection log (INSERT/UPDATE/DELETE with old/new values)
- `snapshots` — point-in-time version markers for time-travel queries

Objects have extracted `location` (PostGIS geometry) and `status` columns for 
fast spatial and status-based filtering, while full properties live in JSONB.

## CURRENT PHASE — WHAT I'M BUILDING FIRST

I am building the **SQL Digestion Engine** as the first component. This is the 
foundation that enables everything else — before ontology mapping, before plugins, 
before the map, I need data flowing into the platform.

The digestion engine:
1. **DISCOVER** — Connect to a source database, introspect all tables, columns, 
   types, primary keys, foreign keys, detect timestamp columns and semantic types
2. **SNAPSHOT** — Full initial copy of all data into platform DB as JSONB rows
3. **TRACK** — Continuously detect changes via timestamp-based incremental sync, 
   full-table hash comparison, or row-level hash diffing
4. **VERSION** — Every sync creates a versioned snapshot enabling time-travel queries
5. **CATALOG** — Maintain registry of all sources, tables, columns, sync status

The engine is a Go library with three interfaces:
- **HTTP API** (Echo) — for frontend and external integrations
- **Background Scheduler** — runs syncs on configured intervals
- **CLI** (future) — for developer convenience

The connector system is interface-based, starting with PostgreSQL driver, 
with MySQL and MSSQL planned.

## WHAT I'VE DECIDED

- Go/Java for all backend services except AI (Python FastAPI for that)
- PostgreSQL as the single data store with PostGIS, AGE, pgvector extensions
- Plugin architecture where the kernel is industry-agnostic
- Ontology-driven UI where display configuration lives in object type metadata
- Shared Context model for inter-plugin communication
- Industry templates as the primary onboarding mechanism
- Warehouses as the first vertical.
- The map is the primary UI for fire department users (MapLibre GL)

## WHAT I HAVE NOT BUILT YET

- Ontology engine (object types, properties, relationships CRUD)
- Plugin SDK and plugin system
- Frontend / Shell / Any UI
- AI/LLM service
- Real-time WebSocket layer
- Authentication/authorization
- Data transformation pipelines


## GUIDING PRINCIPLES

1. The kernel must be 100% industry-agnostic — zero domain knowledge in kernel code
2. PostgreSQL is the single source of truth — minimize infrastructure
3. Build the engine as a library first, then wrap with API — the engine doesn't know about HTTP
4. Stream data from sources using Go channels — don't load entire tables in memory
5. Every data change produces an event — this enables real-time updates later
6. Store ingested data as JSONB with extracted spatial and status columns for fast queries
7. Row hashing enables efficient change detection without requiring source schema changes
8. Ship fast, iterate, don't over-engineer the POC