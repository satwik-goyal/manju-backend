-- ═══════════════════════════════════════════════
-- TABLE: warehouses
-- ═══════════════════════════════════════════════
CREATE TABLE warehouses (
    id              SERIAL PRIMARY KEY,
    warehouse_code  VARCHAR(20) UNIQUE NOT NULL,
    name            VARCHAR(255) NOT NULL,
    address         TEXT,
    city            VARCHAR(100),
    country         VARCHAR(100) DEFAULT 'UAE',
    latitude        DOUBLE PRECISION,
    longitude       DOUBLE PRECISION,
    total_area_sqm  INTEGER,
    storage_capacity_pallets INTEGER,
    current_utilization_pct DECIMAL(5,2) DEFAULT 0,
    type            VARCHAR(50) NOT NULL,
    status          VARCHAR(50) DEFAULT 'operational',
    manager_name    VARCHAR(255),
    phone           VARCHAR(20),
    operating_hours VARCHAR(100) DEFAULT '24/7',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ═══════════════════════════════════════════════
-- TABLE: zones (areas within a warehouse)
-- ═══════════════════════════════════════════════
CREATE TABLE zones (
    id              SERIAL PRIMARY KEY,
    zone_code       VARCHAR(30) UNIQUE NOT NULL,
    warehouse_id    INTEGER NOT NULL REFERENCES warehouses(id),
    name            VARCHAR(255) NOT NULL,
    type            VARCHAR(50) NOT NULL,
    temperature     VARCHAR(50),
    capacity_pallets INTEGER,
    current_pallets INTEGER DEFAULT 0,
    aisle_count     INTEGER,
    rack_levels     INTEGER,
    status          VARCHAR(50) DEFAULT 'active',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ═══════════════════════════════════════════════
-- TABLE: locations (specific rack/bin positions)
-- ═══════════════════════════════════════════════
CREATE TABLE locations (
    id              SERIAL PRIMARY KEY,
    location_code   VARCHAR(30) UNIQUE NOT NULL,
    zone_id         INTEGER NOT NULL REFERENCES zones(id),
    aisle           VARCHAR(10),
    rack            VARCHAR(10),
    level           VARCHAR(10),
    position        VARCHAR(10),
    type            VARCHAR(50) DEFAULT 'pallet',
    max_weight_kg   DECIMAL(10,2),
    max_height_cm   INTEGER,
    is_occupied     BOOLEAN DEFAULT FALSE,
    current_sku     VARCHAR(50),
    status          VARCHAR(50) DEFAULT 'available',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ═══════════════════════════════════════════════
-- TABLE: clients (companies storing goods here)
-- ═══════════════════════════════════════════════
CREATE TABLE clients (
    id              SERIAL PRIMARY KEY,
    client_code     VARCHAR(20) UNIQUE NOT NULL,
    company_name    VARCHAR(255) NOT NULL,
    contact_name    VARCHAR(255),
    email           VARCHAR(255),
    phone           VARCHAR(20),
    address         TEXT,
    city            VARCHAR(100),
    country         VARCHAR(100),
    tax_id          VARCHAR(50),
    contract_start  DATE,
    contract_end    DATE,
    billing_type    VARCHAR(50) DEFAULT 'per_pallet',
    rate_per_pallet DECIMAL(10,2),
    status          VARCHAR(50) DEFAULT 'active',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ═══════════════════════════════════════════════
-- TABLE: products (SKUs stored in the warehouse)
-- ═══════════════════════════════════════════════
CREATE TABLE products (
    id              SERIAL PRIMARY KEY,
    sku             VARCHAR(50) UNIQUE NOT NULL,
    client_id       INTEGER NOT NULL REFERENCES clients(id),
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    category        VARCHAR(100),
    subcategory     VARCHAR(100),
    unit_of_measure VARCHAR(20) DEFAULT 'each',
    units_per_case  INTEGER,
    cases_per_pallet INTEGER,
    weight_kg       DECIMAL(10,3),
    length_cm       DECIMAL(8,2),
    width_cm        DECIMAL(8,2),
    height_cm       DECIMAL(8,2),
    is_hazardous    BOOLEAN DEFAULT FALSE,
    requires_cold   BOOLEAN DEFAULT FALSE,
    min_temperature DECIMAL(5,2),
    max_temperature DECIMAL(5,2),
    shelf_life_days INTEGER,
    barcode         VARCHAR(50),
    status          VARCHAR(50) DEFAULT 'active',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ═══════════════════════════════════════════════
-- TABLE: inventory (current stock levels)
-- ═══════════════════════════════════════════════
CREATE TABLE inventory (
    id              SERIAL PRIMARY KEY,
    product_id      INTEGER NOT NULL REFERENCES products(id),
    location_id     INTEGER NOT NULL REFERENCES locations(id),
    warehouse_id    INTEGER NOT NULL REFERENCES warehouses(id),
    lot_number      VARCHAR(50),
    batch_number    VARCHAR(50),
    quantity        INTEGER NOT NULL DEFAULT 0,
    quantity_reserved INTEGER DEFAULT 0,
    quantity_available INTEGER GENERATED ALWAYS AS (quantity - quantity_reserved) STORED,
    expiry_date     DATE,
    received_date   DATE,
    unit_cost       DECIMAL(10,2),
    status          VARCHAR(50) DEFAULT 'available',
    last_counted_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(product_id, location_id, lot_number)
);

-- ═══════════════════════════════════════════════
-- TABLE: inbound_orders (receiving / purchase orders)
-- ═══════════════════════════════════════════════
CREATE TABLE inbound_orders (
    id              SERIAL PRIMARY KEY,
    order_number    VARCHAR(30) UNIQUE NOT NULL,
    client_id       INTEGER NOT NULL REFERENCES clients(id),
    warehouse_id    INTEGER NOT NULL REFERENCES warehouses(id),
    supplier_name   VARCHAR(255),

    status          VARCHAR(50) DEFAULT 'expected',
    priority        VARCHAR(20) DEFAULT 'normal',

    expected_date   DATE,
    arrival_date    TIMESTAMPTZ,
    completed_date  TIMESTAMPTZ,

    expected_pallets INTEGER,
    received_pallets INTEGER DEFAULT 0,
    expected_units  INTEGER,
    received_units  INTEGER DEFAULT 0,

    carrier_name    VARCHAR(255),
    truck_plate     VARCHAR(20),
    dock_door       VARCHAR(10),
    po_reference    VARCHAR(50),
    notes           TEXT,

    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ═══════════════════════════════════════════════
-- TABLE: inbound_order_lines (items in an inbound order)
-- ═══════════════════════════════════════════════
CREATE TABLE inbound_order_lines (
    id              SERIAL PRIMARY KEY,
    order_id        INTEGER NOT NULL REFERENCES inbound_orders(id) ON DELETE CASCADE,
    product_id      INTEGER NOT NULL REFERENCES products(id),
    expected_quantity INTEGER NOT NULL,
    received_quantity INTEGER DEFAULT 0,
    damaged_quantity INTEGER DEFAULT 0,
    lot_number      VARCHAR(50),
    expiry_date     DATE,
    location_id     INTEGER REFERENCES locations(id),
    status          VARCHAR(50) DEFAULT 'pending',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ═══════════════════════════════════════════════
-- TABLE: outbound_orders (shipping / sales orders)
-- ═══════════════════════════════════════════════
CREATE TABLE outbound_orders (
    id              SERIAL PRIMARY KEY,
    order_number    VARCHAR(30) UNIQUE NOT NULL,
    client_id       INTEGER NOT NULL REFERENCES clients(id),
    warehouse_id    INTEGER NOT NULL REFERENCES warehouses(id),

    customer_name   VARCHAR(255),
    ship_to_address TEXT,
    ship_to_city    VARCHAR(100),
    ship_to_country VARCHAR(100),

    status          VARCHAR(50) DEFAULT 'pending',
    priority        VARCHAR(20) DEFAULT 'normal',

    order_date      TIMESTAMPTZ,
    required_date   DATE,
    ship_date       TIMESTAMPTZ,
    delivered_date  TIMESTAMPTZ,

    total_lines     INTEGER DEFAULT 0,
    total_units     INTEGER DEFAULT 0,
    total_weight_kg DECIMAL(10,2),

    carrier_name    VARCHAR(255),
    tracking_number VARCHAR(100),
    shipping_method VARCHAR(50),
    dock_door       VARCHAR(10),

    so_reference    VARCHAR(50),
    notes           TEXT,

    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ═══════════════════════════════════════════════
-- TABLE: workers (warehouse staff)
-- *** MUST be created BEFORE outbound_order_lines ***
-- ═══════════════════════════════════════════════
CREATE TABLE workers (
    id              SERIAL PRIMARY KEY,
    employee_id     VARCHAR(20) UNIQUE NOT NULL,
    first_name      VARCHAR(100) NOT NULL,
    last_name       VARCHAR(100) NOT NULL,
    role            VARCHAR(50) NOT NULL,
    warehouse_id    INTEGER REFERENCES warehouses(id),
    assigned_zone_id INTEGER REFERENCES zones(id),
    shift           VARCHAR(20),
    status          VARCHAR(50) DEFAULT 'active',
    phone           VARCHAR(20),
    hire_date       DATE,
    hourly_rate     DECIMAL(8,2),
    certification   VARCHAR(100),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ═══════════════════════════════════════════════
-- TABLE: outbound_order_lines
-- *** Now safe to reference workers ***
-- ═══════════════════════════════════════════════
CREATE TABLE outbound_order_lines (
    id              SERIAL PRIMARY KEY,
    order_id        INTEGER NOT NULL REFERENCES outbound_orders(id) ON DELETE CASCADE,
    product_id      INTEGER NOT NULL REFERENCES products(id),
    ordered_quantity INTEGER NOT NULL,
    allocated_quantity INTEGER DEFAULT 0,
    picked_quantity INTEGER DEFAULT 0,
    shipped_quantity INTEGER DEFAULT 0,
    location_id     INTEGER REFERENCES locations(id),
    lot_number      VARCHAR(50),
    status          VARCHAR(50) DEFAULT 'pending',
    picked_by       INTEGER REFERENCES workers(id),
    picked_at       TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ═══════════════════════════════════════════════
-- TABLE: equipment (forklifts, scanners, etc.)
-- ═══════════════════════════════════════════════
CREATE TABLE equipment (
    id              SERIAL PRIMARY KEY,
    equipment_code  VARCHAR(30) UNIQUE NOT NULL,
    name            VARCHAR(255) NOT NULL,
    type            VARCHAR(50) NOT NULL,
    warehouse_id    INTEGER REFERENCES warehouses(id),
    assigned_zone_id INTEGER REFERENCES zones(id),
    serial_number   VARCHAR(100),
    manufacturer    VARCHAR(100),
    model           VARCHAR(100),
    year            INTEGER,
    status          VARCHAR(50) DEFAULT 'operational',
    last_maintenance DATE,
    next_maintenance DATE,
    hours_used      DECIMAL(10,1) DEFAULT 0,
    fuel_type       VARCHAR(20),
    battery_level   INTEGER,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ═══════════════════════════════════════════════
-- TABLE: dock_doors
-- ═══════════════════════════════════════════════
CREATE TABLE dock_doors (
    id              SERIAL PRIMARY KEY,
    door_code       VARCHAR(10) UNIQUE NOT NULL,
    warehouse_id    INTEGER NOT NULL REFERENCES warehouses(id),
    type            VARCHAR(50) DEFAULT 'standard',
    status          VARCHAR(50) DEFAULT 'available',
    current_order_id INTEGER,
    current_truck   VARCHAR(20),
    last_used_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ═══════════════════════════════════════════════
-- TABLE: inventory_transactions (movement log)
-- ═══════════════════════════════════════════════
CREATE TABLE inventory_transactions (
    id              SERIAL PRIMARY KEY,
    product_id      INTEGER NOT NULL REFERENCES products(id),
    warehouse_id    INTEGER NOT NULL REFERENCES warehouses(id),
    from_location_id INTEGER REFERENCES locations(id),
    to_location_id  INTEGER REFERENCES locations(id),
    transaction_type VARCHAR(50) NOT NULL,
    quantity        INTEGER NOT NULL,
    lot_number      VARCHAR(50),
    reference_type  VARCHAR(50),
    reference_id    INTEGER,
    worker_id       INTEGER REFERENCES workers(id),
    notes           TEXT,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ═══════════════════════════════════════════════
-- SEED DATA: Warehouses
-- ═══════════════════════════════════════════════
INSERT INTO warehouses (warehouse_code, name, address, city, country, latitude, longitude, total_area_sqm, storage_capacity_pallets, current_utilization_pct, type, status, manager_name, phone) VALUES
('WH-JA-01', 'Jebel Ali Distribution Center',     'Jebel Ali Free Zone, Plot 2847',       'Dubai',     'UAE',    25.0185, 55.0820, 15000, 8500,  78.5, 'ambient',      'operational', 'Khalid Al Maktoum',   '+971-4-880-0101'),
('WH-JA-02', 'Jebel Ali Cold Storage',            'Jebel Ali Free Zone, Plot 3120',       'Dubai',     'UAE',    25.0200, 55.0750, 5000,  2200,  91.2, 'cold_storage', 'operational', 'Ahmed Hassan',        '+971-4-880-0102'),
('WH-DIP-01','Dubai Investment Park Warehouse',    'DIP 2, Building 14',                   'Dubai',     'UAE',    24.9850, 55.1520, 8000,  4000,  65.3, 'ambient',      'operational', 'Omar Al Rashid',      '+971-4-880-0103'),
('WH-SHJ-01','Sharjah Logistics Hub',             'Sharjah Airport Free Zone, Blk C',     'Sharjah',   'UAE',    25.3200, 55.5100, 6000,  3200,  54.8, 'ambient',      'operational', 'Saeed Al Shamsi',     '+971-6-550-0201'),
('WH-AUH-01','Abu Dhabi KIZAD Facility',          'KIZAD Logistics Park, Zone A',         'Abu Dhabi', 'UAE',    24.5900, 54.6500, 10000, 5500,  42.1, 'ambient',      'under_construction', 'Faisal Al Dhaheri', '+971-2-510-0301');

-- ═══════════════════════════════════════════════
-- SEED DATA: Zones
-- ═══════════════════════════════════════════════
INSERT INTO zones (zone_code, warehouse_id, name, type, temperature, capacity_pallets, current_pallets, aisle_count, rack_levels, status) VALUES
-- Jebel Ali DC
('JA01-RCV',  1, 'Receiving Area',      'receiving',  'ambient',  200,  45,  NULL, NULL, 'active'),
('JA01-STR-A',1, 'Storage Zone A',      'storage',    'ambient',  3000, 2450, 12,   5,   'active'),
('JA01-STR-B',1, 'Storage Zone B',      'storage',    'ambient',  3000, 2280, 12,   5,   'active'),
('JA01-STR-C',1, 'Storage Zone C - High Value', 'storage', 'ambient', 1500, 1100, 6, 4,  'active'),
('JA01-PCK',  1, 'Picking Zone',        'picking',    'ambient',  500,  380,  8,    3,   'active'),
('JA01-PAK',  1, 'Packing Area',        'packing',    'ambient',  100,  35,   NULL, NULL, 'active'),
('JA01-SHP',  1, 'Shipping Staging',    'shipping',   'ambient',  200,  85,   NULL, NULL, 'active'),
-- Jebel Ali Cold Storage
('JA02-RCV',  2, 'Cold Receiving',      'receiving',  'chilled_2_8',    50,  12, NULL, NULL, 'active'),
('JA02-CHL',  2, 'Chilled Storage',     'storage',    'chilled_2_8',    1200, 1050, 6, 4,  'active'),
('JA02-FRZ',  2, 'Frozen Storage',      'storage',    'frozen_minus_18', 800, 752, 4, 4,   'active'),
('JA02-SHP',  2, 'Cold Shipping',       'shipping',   'chilled_2_8',    150,  42, NULL, NULL, 'active'),
-- DIP Warehouse
('DIP01-RCV', 3, 'Receiving Bay',       'receiving',  'ambient',  100,  28,  NULL, NULL, 'active'),
('DIP01-STR', 3, 'Main Storage',        'storage',    'ambient',  3200, 2100, 10, 5,    'active'),
('DIP01-SHP', 3, 'Shipping Area',       'shipping',   'ambient',  150,  55,  NULL, NULL, 'active'),
('DIP01-RET', 3, 'Returns Processing',  'returns',    'ambient',  200,  78,  NULL, NULL, 'active'),
-- Sharjah Hub
('SHJ01-STR', 4, 'Main Storage',        'storage',    'ambient',  2800, 1520, 8, 5,     'active'),
('SHJ01-XDK', 4, 'Cross-Dock Area',     'staging',    'ambient',  400,  180,  NULL, NULL, 'active');

-- ═══════════════════════════════════════════════
-- SEED DATA: Clients
-- ═══════════════════════════════════════════════
INSERT INTO clients (client_code, company_name, contact_name, email, phone, address, city, country, tax_id, contract_start, contract_end, billing_type, rate_per_pallet, status) VALUES
('CL-001', 'Emirates FMCG Trading',        'Fatima Al Zaabi',  'fatima@emirates-fmcg.ae',    '+971-4-333-0001', 'Business Bay Tower 3',     'Dubai',     'UAE',    'TRN100001', '2023-01-01', '2025-12-31', 'per_pallet',    12.50, 'active'),
('CL-002', 'Gulf Electronics LLC',         'Ravi Sharma',      'ravi@gulfelectronics.ae',    '+971-4-333-0002', 'DAFZA Office 204',         'Dubai',     'UAE',    'TRN100002', '2023-06-01', '2025-05-31', 'per_pallet',    15.00, 'active'),
('CL-003', 'Al Safa Foods International',  'Noor Al Hashimi',  'noor@alsafafoods.ae',        '+971-4-333-0003', 'Jumeirah Lake Towers',     'Dubai',     'UAE',    'TRN100003', '2022-03-15', '2025-03-14', 'per_pallet',    18.00, 'active'),
('CL-004', 'Desert Pharma Distribution',   'Dr. Aisha Mirza',  'aisha@desertpharma.ae',      '+971-4-333-0004', 'Healthcare City Bldg 47',  'Dubai',     'UAE',    'TRN100004', '2024-01-01', '2026-12-31', 'fixed_monthly', NULL,  'active'),
('CL-005', 'Zenith Auto Parts FZE',        'Vikram Patel',     'vikram@zenithauto.ae',       '+971-6-333-0005', 'SAIF Zone, Block D',       'Sharjah',   'UAE',    'TRN100005', '2023-09-01', '2025-08-31', 'per_pallet',    10.00, 'active'),
('CL-006', 'Oasis Home & Garden',          'Sara Thompson',    'sara@oasishome.ae',          '+971-4-333-0006', 'Al Quoz Industrial',       'Dubai',     'UAE',    'TRN100006', '2024-03-01', '2026-02-28', 'per_sqm',       NULL,  'active'),
('CL-007', 'Atlas Sportswear DMCC',        'James Chen',       'james@atlassport.ae',        '+971-4-333-0007', 'JLT Cluster Y',            'Dubai',     'UAE',    'TRN100007', '2023-11-01', '2025-10-31', 'per_pallet',    13.00, 'active'),
('CL-008', 'Crescent Cosmetics',           'Layla Ahmad',      'layla@crescentcos.ae',       '+971-4-333-0008', 'Design District D3',       'Dubai',     'UAE',    'TRN100008', '2024-06-01', '2026-05-31', 'per_pallet',    16.00, 'active');

-- ═══════════════════════════════════════════════
-- SEED DATA: Products (30 SKUs across clients)
-- ═══════════════════════════════════════════════
INSERT INTO products (sku, client_id, name, description, category, subcategory, unit_of_measure, units_per_case, cases_per_pallet, weight_kg, length_cm, width_cm, height_cm, is_hazardous, requires_cold, shelf_life_days, barcode, status) VALUES
-- Emirates FMCG
('FMCG-WTR-500',  1, 'Mineral Water 500ml',         '24-pack shrink wrap',         'Beverages',     'Water',          'case',  24, 80,  12.5, 40, 27, 22, FALSE, FALSE, 365, '6291000000101', 'active'),
('FMCG-JCE-1L',   1, 'Orange Juice 1L',             '12-pack carton',              'Beverages',     'Juice',          'case',  12, 60,  13.2, 40, 30, 28, FALSE, TRUE,  90,  '6291000000102', 'active'),
('FMCG-RCE-5KG',  1, 'Basmati Rice 5kg',            '4 bags per case',             'Dry Goods',     'Rice',           'case',  4,  48,  21.0, 45, 30, 35, FALSE, FALSE, 720, '6291000000103', 'active'),
('FMCG-OIL-2L',   1, 'Vegetable Oil 2L',            '6 bottles per case',          'Dry Goods',     'Oil',            'case',  6,  40,  13.5, 35, 25, 30, FALSE, FALSE, 540, '6291000000104', 'active'),
-- Gulf Electronics
('ELEC-PHN-S24',   2, 'Smartphone Model S24',        'Latest flagship phone',       'Electronics',   'Phones',         'each',  1,  100, 0.23, 16, 8,  8,  FALSE, FALSE, NULL,'6291000000201', 'active'),
('ELEC-TAB-A10',   2, 'Tablet A10 WiFi',             '10.5 inch tablet',            'Electronics',   'Tablets',        'each',  1,  60,  0.52, 25, 18, 8,  FALSE, FALSE, NULL,'6291000000202', 'active'),
('ELEC-EBD-PRO',   2, 'Wireless Earbuds Pro',        'ANC earbuds with case',       'Electronics',   'Audio',          'each',  1,  200, 0.06, 8,  8,  4,  FALSE, FALSE, NULL,'6291000000203', 'active'),
('ELEC-CHG-65W',   2, 'USB-C Charger 65W',           'Fast charger',                'Electronics',   'Accessories',    'each',  1,  300, 0.12, 10, 6,  3,  FALSE, FALSE, NULL,'6291000000204', 'active'),
-- Al Safa Foods (cold chain)
('FOOD-CHK-1KG',   3, 'Frozen Chicken Breast 1kg',   'IQF chicken breast',          'Frozen Foods',  'Poultry',        'case',  10, 40,  11.0, 40, 30, 25, FALSE, TRUE,  365, '6291000000301', 'active'),
('FOOD-ICE-1L',    3, 'Vanilla Ice Cream 1L',        'Premium ice cream',           'Frozen Foods',  'Ice Cream',      'case',  8,  36,  9.0,  35, 25, 20, FALSE, TRUE,  180, '6291000000302', 'active'),
('FOOD-YGT-500',   3, 'Greek Yogurt 500g',           '6-pack',                      'Dairy',         'Yogurt',         'case',  6,  48,  3.5,  30, 20, 12, FALSE, TRUE,  30,  '6291000000303', 'active'),
('FOOD-CHS-200',   3, 'Cheddar Cheese 200g',         '12-pack sliced',              'Dairy',         'Cheese',         'case',  12, 60,  2.8,  25, 20, 15, FALSE, TRUE,  90,  '6291000000304', 'active'),
-- Desert Pharma
('PHRM-VIT-C',     4, 'Vitamin C 1000mg',            '60 tablets per bottle',       'Supplements',   'Vitamins',       'case',  24, 80,  4.2,  30, 25, 20, FALSE, FALSE, 730, '6291000000401', 'active'),
('PHRM-MSK-N95',   4, 'N95 Face Masks',              '20 masks per box',            'Medical',       'PPE',            'case',  50, 40,  8.5,  45, 35, 30, FALSE, FALSE, NULL,'6291000000402', 'active'),
('PHRM-SAN-500',   4, 'Hand Sanitizer 500ml',        '24 bottles per case',         'Medical',       'Sanitization',   'case',  24, 36,  13.0, 40, 30, 25, TRUE,  FALSE, 730, '6291000000403', 'active'),
-- Zenith Auto Parts
('AUTO-FLT-OIL',   5, 'Oil Filter Universal',        'Fits most sedan models',      'Auto Parts',    'Filters',        'each',  1,  200, 0.35, 12, 12, 10, FALSE, FALSE, NULL,'6291000000501', 'active'),
('AUTO-BRK-PAD',   5, 'Brake Pad Set Front',         'Ceramic brake pads',          'Auto Parts',    'Brakes',         'each',  1,  120, 1.20, 18, 15, 5,  FALSE, FALSE, NULL,'6291000000502', 'active'),
('AUTO-BAT-12V',   5, 'Car Battery 12V 60Ah',        'Maintenance-free battery',    'Auto Parts',    'Electrical',     'each',  1,  24,  15.5, 24, 18, 20, TRUE,  FALSE, NULL,'6291000000503', 'active'),
-- Oasis Home & Garden
('HOME-PLW-STD',   6, 'Memory Foam Pillow Standard', 'Medium firmness',             'Home',          'Bedding',        'each',  1,  40,  1.20, 65, 45, 15, FALSE, FALSE, NULL,'6291000000601', 'active'),
('HOME-POT-LG',    6, 'Ceramic Plant Pot Large',     '30cm diameter',               'Garden',        'Pots',           'each',  1,  24,  3.50, 32, 32, 28, FALSE, FALSE, NULL,'6291000000602', 'active'),
-- Atlas Sportswear
('SPRT-TSH-M',     7, 'Performance T-Shirt Mens',    'Moisture-wicking fabric',     'Apparel',       'Tops',           'each',  1,  200, 0.18, 30, 25, 3,  FALSE, FALSE, NULL,'6291000000701', 'active'),
('SPRT-SHO-RUN',   7, 'Running Shoes Unisex',        'Cushioned running shoe',      'Footwear',      'Running',        'each',  1,  40,  0.65, 35, 22, 14, FALSE, FALSE, NULL,'6291000000702', 'active'),
-- Crescent Cosmetics
('COSM-CRM-50',    8, 'Anti-Aging Face Cream 50ml',  'Premium skincare',            'Cosmetics',     'Skincare',       'each',  1,  300, 0.08, 6,  6,  6,  FALSE, FALSE, 730, '6291000000801', 'active'),
('COSM-PFM-100',   8, 'Signature Perfume 100ml',     'Eau de parfum',               'Cosmetics',     'Fragrance',      'each',  1,  120, 0.35, 15, 8,  8,  TRUE,  FALSE, NULL,'6291000000802', 'active');

-- ═══════════════════════════════════════════════
-- SEED DATA: Workers
-- ═══════════════════════════════════════════════
INSERT INTO workers (employee_id, first_name, last_name, role, warehouse_id, assigned_zone_id, shift, status, phone, hire_date, hourly_rate, certification) VALUES
('WRK-001', 'Rajesh',   'Kumar',       'supervisor',        1, NULL, 'morning',   'active',    '+971-55-100-0001', '2020-01-15', 45.00, 'forklift,hazmat'),
('WRK-002', 'Abdul',    'Rahman',      'forklift_operator', 1, 2,    'morning',   'active',    '+971-55-100-0002', '2021-03-10', 30.00, 'forklift'),
('WRK-003', 'Deepak',   'Singh',       'forklift_operator', 1, 3,    'morning',   'active',    '+971-55-100-0003', '2021-06-20', 30.00, 'forklift'),
('WRK-004', 'Ali',      'Hassan',      'picker',            1, 5,    'morning',   'active',    '+971-55-100-0004', '2022-01-05', 22.00, NULL),
('WRK-005', 'Pradeep',  'Nair',        'picker',            1, 5,    'morning',   'active',    '+971-55-100-0005', '2022-04-18', 22.00, NULL),
('WRK-006', 'Mohammed', 'Ismail',      'picker',            1, 5,    'afternoon', 'active',    '+971-55-100-0006', '2022-08-01', 22.00, NULL),
('WRK-007', 'Sanjay',   'Patel',       'packer',            1, 6,    'morning',   'active',    '+971-55-100-0007', '2023-02-14', 20.00, NULL),
('WRK-008', 'Imran',    'Khan',        'receiver',          1, 1,    'morning',   'active',    '+971-55-100-0008', '2021-09-01', 25.00, 'forklift'),
('WRK-009', 'Suresh',   'Menon',       'supervisor',        2, NULL, 'morning',   'active',    '+971-55-100-0009', '2019-05-20', 48.00, 'forklift,cold_chain'),
('WRK-010', 'Tariq',    'Al Balushi',  'forklift_operator', 2, 9,    'morning',   'active',    '+971-55-100-0010', '2022-11-10', 32.00, 'forklift,cold_chain'),
('WRK-011', 'Vinod',    'George',      'picker',            2, 9,    'morning',   'active',    '+971-55-100-0011', '2023-05-01', 24.00, 'cold_chain'),
('WRK-012', 'Nasir',    'Ahmed',       'receiver',          3, 12,   'morning',   'active',    '+971-55-100-0012', '2022-07-15', 25.00, 'forklift'),
('WRK-013', 'Ramesh',   'Babu',        'picker',            3, 13,   'morning',   'on_break',  '+971-55-100-0013', '2023-08-20', 22.00, NULL),
('WRK-014', 'Yousuf',   'Al Marzouqi', 'supervisor',        4, NULL, 'morning',   'active',    '+971-55-100-0014', '2020-10-01', 42.00, 'forklift'),
('WRK-015', 'Ganesh',   'Pillai',      'forklift_operator', 4, 16,   'morning',   'active',    '+971-55-100-0015', '2023-01-10', 28.00, 'forklift');

-- ═══════════════════════════════════════════════
-- SEED DATA: Locations (sample bin locations)
-- ═══════════════════════════════════════════════
INSERT INTO locations (location_code, zone_id, aisle, rack, level, position, type, max_weight_kg, max_height_cm, is_occupied, current_sku, status) VALUES
-- Zone A locations (WH-JA-01)
('A-01-01-01', 2, 'A-01', 'R01', 'L1', NULL, 'pallet', 1200, 180, TRUE,  'FMCG-WTR-500', 'occupied'),
('A-01-01-02', 2, 'A-01', 'R01', 'L2', NULL, 'pallet', 1000, 150, TRUE,  'FMCG-WTR-500', 'occupied'),
('A-01-01-03', 2, 'A-01', 'R01', 'L3', NULL, 'pallet', 800,  120, FALSE, NULL,            'available'),
('A-01-02-01', 2, 'A-01', 'R02', 'L1', NULL, 'pallet', 1200, 180, TRUE,  'FMCG-RCE-5KG', 'occupied'),
('A-01-02-02', 2, 'A-01', 'R02', 'L2', NULL, 'pallet', 1000, 150, TRUE,  'FMCG-RCE-5KG', 'occupied'),
('A-02-01-01', 2, 'A-02', 'R01', 'L1', NULL, 'pallet', 1200, 180, TRUE,  'FMCG-OIL-2L',  'occupied'),
('A-02-01-02', 2, 'A-02', 'R01', 'L2', NULL, 'pallet', 1000, 150, FALSE, NULL,            'available'),
-- Zone B locations
('B-01-01-01', 3, 'B-01', 'R01', 'L1', NULL, 'pallet', 1200, 180, TRUE,  'ELEC-PHN-S24',  'occupied'),
('B-01-01-02', 3, 'B-01', 'R01', 'L2', NULL, 'pallet', 1000, 150, TRUE,  'ELEC-TAB-A10',  'occupied'),
('B-01-02-01', 3, 'B-01', 'R02', 'L1', NULL, 'pallet', 1200, 180, TRUE,  'ELEC-EBD-PRO',  'occupied'),
('B-02-01-01', 3, 'B-02', 'R01', 'L1', NULL, 'pallet', 1200, 180, TRUE,  'AUTO-FLT-OIL',  'occupied'),
('B-02-01-02', 3, 'B-02', 'R01', 'L2', NULL, 'pallet', 1000, 150, TRUE,  'AUTO-BRK-PAD',  'occupied'),
-- Cold storage locations
('CHL-01-01',  9,  'C-01', 'R01', 'L1', NULL, 'pallet', 1000, 150, TRUE,  'FOOD-YGT-500',  'occupied'),
('CHL-01-02',  9,  'C-01', 'R01', 'L2', NULL, 'pallet', 1000, 150, TRUE,  'FOOD-CHS-200',  'occupied'),
('CHL-02-01',  9,  'C-02', 'R01', 'L1', NULL, 'pallet', 1000, 150, TRUE,  'FMCG-JCE-1L',   'occupied'),
('FRZ-01-01',  10, 'F-01', 'R01', 'L1', NULL, 'pallet', 1000, 150, TRUE,  'FOOD-CHK-1KG',  'occupied'),
('FRZ-01-02',  10, 'F-01', 'R01', 'L2', NULL, 'pallet', 1000, 150, TRUE,  'FOOD-ICE-1L',   'occupied');

-- ═══════════════════════════════════════════════
-- SEED DATA: Inventory
-- ═══════════════════════════════════════════════
INSERT INTO inventory (product_id, location_id, warehouse_id, lot_number, batch_number, quantity, quantity_reserved, expiry_date, received_date, unit_cost, status) VALUES
(1,  1,  1, 'LOT-2024-0901', 'B001', 80,  10, '2025-09-01', '2024-09-01', 3.50,  'available'),
(1,  2,  1, 'LOT-2024-0901', 'B001', 80,  0,  '2025-09-01', '2024-09-01', 3.50,  'available'),
(3,  4,  1, 'LOT-2024-0815', 'B002', 48,  5,  '2026-08-15', '2024-08-15', 12.00, 'available'),
(3,  5,  1, 'LOT-2024-0815', 'B002', 48,  0,  '2026-08-15', '2024-08-15', 12.00, 'available'),
(4,  6,  1, 'LOT-2024-0820', 'B003', 40,  8,  '2026-02-20', '2024-08-20', 8.75,  'available'),
(5,  8,  1, 'LOT-2024-0910', 'B004', 100, 15, NULL,         '2024-09-10', 850.00,'available'),
(6,  9,  1, 'LOT-2024-0912', 'B005', 55,  3,  NULL,         '2024-09-12', 420.00,'available'),
(7,  10, 1, 'LOT-2024-0905', 'B006', 200, 0,  NULL,         '2024-09-05', 125.00,'available');
