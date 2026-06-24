package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Store struct{ pool *pgxpool.Pool }

func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// WithLeaderLock runs fn only if this process can immediately acquire the given
// Postgres session-level advisory lock. It returns ran=false (and does not call
// fn) when another instance already holds the lock. The lock is held on a
// dedicated connection for fn's duration and released afterward, so across
// multiple API replicas only one evaluates a given tick — alerts aren't sent N
// times. fn may freely use the pool; the lock is just a cross-process mutex.
func (s *Store) WithLeaderLock(ctx context.Context, key int64, fn func(context.Context) error) (ran bool, err error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Release()

	var got bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&got); err != nil {
		return false, err
	}
	if !got {
		return false, nil
	}
	defer func() {
		// Release on a fresh context so a canceled/expired tick still unlocks.
		rctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(rctx, `SELECT pg_advisory_unlock($1)`, key)
	}()
	return true, fn(ctx)
}

// ---------- models ----------

type Organization struct {
	ID                   uuid.UUID  `json:"id"`
	Name                 string     `json:"name"`
	StripeCustomerID     *string    `json:"-"`
	StripeSubscriptionID *string    `json:"-"`
	Plan                 *string    `json:"plan"`
	SubscriptionStatus   string     `json:"subscription_status"`
	TrialEnd             *time.Time `json:"trial_end"`
	CurrentPeriodEnd     *time.Time `json:"current_period_end"`
}

type User struct {
	ID    uuid.UUID `json:"id"`
	OrgID uuid.UUID `json:"org_id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
	Role  string    `json:"role"`
}

type Location struct {
	ID        uuid.UUID `json:"id"`
	OrgID     uuid.UUID `json:"org_id"`
	Name      string    `json:"name"`
	Timezone  string    `json:"timezone"`
	UnitCount int       `json:"unit_count"`
	CreatedAt time.Time `json:"created_at"`
}

type Unit struct {
	ID                 uuid.UUID `json:"id"`
	LocationID         uuid.UUID `json:"location_id"`
	Name               string    `json:"name"`
	Kind               string    `json:"kind"`
	MinTempF           float64   `json:"min_temp_f"`
	MaxTempF           float64   `json:"max_temp_f"`
	LogIntervalMinutes int       `json:"log_interval_minutes"`
	SensorMAC          *string   `json:"sensor_mac"`
}

type Reading struct {
	ID         uuid.UUID  `json:"id"`
	UnitID     uuid.UUID  `json:"unit_id"`
	UnitName   string     `json:"unit_name"`
	TempF      float64    `json:"temp_f"`
	Source     string     `json:"source"`
	Note       string     `json:"note"`
	RecordedBy *string    `json:"recorded_by"`
	RecordedAt time.Time  `json:"recorded_at"`
}

// ---------- auth / users ----------

func (s *Store) CreateOrgWithAdmin(ctx context.Context, orgName, name, email, hash string) (User, Organization, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, Organization{}, err
	}
	defer tx.Rollback(ctx)

	var org Organization
	if err := tx.QueryRow(ctx,
		`INSERT INTO organizations (name, subscription_status, trial_end)
		 VALUES ($1, 'trialing', now() + interval '14 days')
		 RETURNING id, name, plan, subscription_status, trial_end, current_period_end`, orgName,
	).Scan(&org.ID, &org.Name, &org.Plan, &org.SubscriptionStatus, &org.TrialEnd, &org.CurrentPeriodEnd); err != nil {
		return User{}, Organization{}, err
	}

	var u User
	if err := tx.QueryRow(ctx,
		`INSERT INTO users (org_id, email, name, password_hash, role)
		 VALUES ($1, $2, $3, $4, 'admin')
		 RETURNING id, org_id, email, name, role`,
		org.ID, email, name, hash,
	).Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.Role); err != nil {
		return User{}, Organization{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return User{}, Organization{}, err
	}
	return u, org, nil
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, string, error) {
	var u User
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, email, name, role, password_hash FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.Role, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrNotFound
	}
	return u, hash, err
}

func (s *Store) UserByID(ctx context.Context, id uuid.UUID) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, email, name, role FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) OrgByID(ctx context.Context, id uuid.UUID) (Organization, error) {
	return scanOrg(s.pool.QueryRow(ctx, orgSelect+` WHERE id = $1`, id))
}

func (s *Store) OrgByStripeCustomer(ctx context.Context, customerID string) (Organization, error) {
	return scanOrg(s.pool.QueryRow(ctx, orgSelect+` WHERE stripe_customer_id = $1`, customerID))
}

const orgSelect = `SELECT id, name, stripe_customer_id, stripe_subscription_id, plan,
	subscription_status, trial_end, current_period_end FROM organizations`

type rowScanner interface{ Scan(dest ...any) error }

func scanOrg(row rowScanner) (Organization, error) {
	var o Organization
	err := row.Scan(&o.ID, &o.Name, &o.StripeCustomerID, &o.StripeSubscriptionID, &o.Plan,
		&o.SubscriptionStatus, &o.TrialEnd, &o.CurrentPeriodEnd)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	return o, err
}

func (s *Store) SetOrgStripeCustomer(ctx context.Context, orgID uuid.UUID, customerID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE organizations SET stripe_customer_id=$2 WHERE id=$1`, orgID, customerID)
	return err
}

// UpdateOrgSubscription syncs subscription state from a Stripe webhook, keyed by
// the customer id.
func (s *Store) UpdateOrgSubscription(ctx context.Context, customerID, subID, status, plan string, periodEnd *time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE organizations
		 SET stripe_subscription_id=$2, subscription_status=$3, plan=$4, current_period_end=$5
		 WHERE stripe_customer_id=$1`,
		customerID, nullify(subID), status, nullify(plan), periodEnd)
	return err
}

// ---------- locations ----------

func (s *Store) ListLocations(ctx context.Context, orgID uuid.UUID) ([]Location, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT l.id, l.org_id, l.name, l.timezone, l.created_at,
		        COALESCE(count(u.id), 0)
		 FROM locations l
		 LEFT JOIN units u ON u.location_id = l.id
		 WHERE l.org_id = $1
		 GROUP BY l.id
		 ORDER BY l.name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Location{}
	for rows.Next() {
		var l Location
		if err := rows.Scan(&l.ID, &l.OrgID, &l.Name, &l.Timezone, &l.CreatedAt, &l.UnitCount); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) GetLocation(ctx context.Context, orgID, id uuid.UUID) (Location, error) {
	var l Location
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, name, timezone, created_at FROM locations WHERE id = $1 AND org_id = $2`,
		id, orgID,
	).Scan(&l.ID, &l.OrgID, &l.Name, &l.Timezone, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Location{}, ErrNotFound
	}
	return l, err
}

func (s *Store) CreateLocation(ctx context.Context, orgID uuid.UUID, name, tz string) (Location, error) {
	if tz == "" {
		tz = "America/Los_Angeles"
	}
	var l Location
	err := s.pool.QueryRow(ctx,
		`INSERT INTO locations (org_id, name, timezone) VALUES ($1, $2, $3)
		 RETURNING id, org_id, name, timezone, created_at`,
		orgID, name, tz,
	).Scan(&l.ID, &l.OrgID, &l.Name, &l.Timezone, &l.CreatedAt)
	return l, err
}

// CountLocations is used to set the per-location subscription quantity.
func (s *Store) CountLocations(ctx context.Context, orgID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM locations WHERE org_id = $1`, orgID).Scan(&n)
	return n, err
}

// ---------- units ----------

func (s *Store) ListUnits(ctx context.Context, orgID, locationID uuid.UUID) ([]Unit, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, location_id, name, kind, min_temp_f, max_temp_f, log_interval_minutes, sensor_mac
		 FROM units WHERE org_id = $1 AND location_id = $2 ORDER BY name`,
		orgID, locationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Unit{}
	for rows.Next() {
		var u Unit
		if err := rows.Scan(&u.ID, &u.LocationID, &u.Name, &u.Kind, &u.MinTempF, &u.MaxTempF, &u.LogIntervalMinutes, &u.SensorMAC); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) CreateUnit(ctx context.Context, orgID, locationID uuid.UUID, name, kind string, minF, maxF float64, interval int) (Unit, error) {
	if interval <= 0 {
		interval = 240
	}
	var u Unit
	err := s.pool.QueryRow(ctx,
		`INSERT INTO units (org_id, location_id, name, kind, min_temp_f, max_temp_f, log_interval_minutes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, location_id, name, kind, min_temp_f, max_temp_f, log_interval_minutes, sensor_mac`,
		orgID, locationID, name, kind, minF, maxF, interval,
	).Scan(&u.ID, &u.LocationID, &u.Name, &u.Kind, &u.MinTempF, &u.MaxTempF, &u.LogIntervalMinutes, &u.SensorMAC)
	return u, err
}

// unitBelongs confirms a unit is owned by the org (guards cross-tenant writes).
func (s *Store) unitBelongs(ctx context.Context, orgID, unitID uuid.UUID) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM units WHERE id = $1 AND org_id = $2)`, unitID, orgID,
	).Scan(&exists)
	return exists, err
}

// ---------- readings ----------

func (s *Store) CreateReading(ctx context.Context, orgID, unitID, recordedBy uuid.UUID, tempF float64, note string) (Reading, error) {
	ok, err := s.unitBelongs(ctx, orgID, unitID)
	if err != nil {
		return Reading{}, err
	}
	if !ok {
		return Reading{}, ErrNotFound
	}
	rb := recordedBy
	return s.insertChainedReading(ctx, orgID, unitID, tempF, "manual", note, &rb, nil)
}

// ListReadings returns readings for a location within [from, to].
func (s *Store) ListReadings(ctx context.Context, orgID, locationID uuid.UUID, from, to time.Time) ([]Reading, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT r.id, r.unit_id, u.name, r.temp_f, r.source, COALESCE(r.note, ''),
		        usr.name, r.recorded_at
		 FROM readings r
		 JOIN units u ON u.id = r.unit_id
		 LEFT JOIN users usr ON usr.id = r.recorded_by
		 WHERE r.org_id = $1 AND u.location_id = $2
		   AND r.recorded_at >= $3 AND r.recorded_at < $4
		 ORDER BY r.recorded_at DESC`,
		orgID, locationID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Reading{}
	for rows.Next() {
		var r Reading
		var by *string
		if err := rows.Scan(&r.ID, &r.UnitID, &r.UnitName, &r.TempF, &r.Source, &r.Note, &by, &r.RecordedAt); err != nil {
			return nil, err
		}
		r.RecordedBy = by
		out = append(out, r)
	}
	return out, rows.Err()
}

// LatestReading is the most recent reading for a unit (used by the dashboard).
type LatestReading struct {
	TempF      float64
	RecordedAt time.Time
	By         *string
}

func (s *Store) LatestByLocation(ctx context.Context, orgID, locationID uuid.UUID) (map[uuid.UUID]LatestReading, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT ON (r.unit_id) r.unit_id, r.temp_f, r.recorded_at, usr.name
		 FROM readings r
		 JOIN units u ON u.id = r.unit_id
		 LEFT JOIN users usr ON usr.id = r.recorded_by
		 WHERE r.org_id = $1 AND u.location_id = $2
		 ORDER BY r.unit_id, r.recorded_at DESC`,
		orgID, locationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[uuid.UUID]LatestReading{}
	for rows.Next() {
		var id uuid.UUID
		var lr LatestReading
		if err := rows.Scan(&id, &lr.TempF, &lr.RecordedAt, &lr.By); err != nil {
			return nil, err
		}
		out[id] = lr
	}
	return out, rows.Err()
}

func nullify(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ---------- gateways & sensor ingest ----------

type Gateway struct {
	ID         uuid.UUID  `json:"id"`
	OrgID      uuid.UUID  `json:"org_id"`
	LocationID uuid.UUID  `json:"location_id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (s *Store) CreateGateway(ctx context.Context, orgID, locationID uuid.UUID, name, keyHash, keyPrefix string) (Gateway, error) {
	var g Gateway
	err := s.pool.QueryRow(ctx,
		`INSERT INTO gateways (org_id, location_id, name, api_key_hash, key_prefix)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, org_id, location_id, name, key_prefix, last_seen_at, created_at`,
		orgID, locationID, name, keyHash, keyPrefix,
	).Scan(&g.ID, &g.OrgID, &g.LocationID, &g.Name, &g.KeyPrefix, &g.LastSeenAt, &g.CreatedAt)
	return g, err
}

func (s *Store) ListGateways(ctx context.Context, orgID, locationID uuid.UUID) ([]Gateway, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, org_id, location_id, name, key_prefix, last_seen_at, created_at
		 FROM gateways WHERE org_id = $1 AND location_id = $2 ORDER BY created_at`,
		orgID, locationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Gateway{}
	for rows.Next() {
		var g Gateway
		if err := rows.Scan(&g.ID, &g.OrgID, &g.LocationID, &g.Name, &g.KeyPrefix, &g.LastSeenAt, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GatewayByKeyHash resolves an ingest request to its owning org and location.
func (s *Store) GatewayByKeyHash(ctx context.Context, keyHash string) (Gateway, error) {
	var g Gateway
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, location_id, name, key_prefix, last_seen_at, created_at
		 FROM gateways WHERE api_key_hash = $1`, keyHash,
	).Scan(&g.ID, &g.OrgID, &g.LocationID, &g.Name, &g.KeyPrefix, &g.LastSeenAt, &g.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Gateway{}, ErrNotFound
	}
	return g, err
}

func (s *Store) TouchGateway(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE gateways SET last_seen_at = now() WHERE id = $1`, id)
	return err
}

// UnitByMAC finds the unit a sensor MAC is bound to, within an org.
func (s *Store) UnitByMAC(ctx context.Context, orgID uuid.UUID, mac string) (Unit, error) {
	var u Unit
	err := s.pool.QueryRow(ctx,
		`SELECT id, location_id, name, kind, min_temp_f, max_temp_f, log_interval_minutes, sensor_mac
		 FROM units WHERE org_id = $1 AND sensor_mac = $2`, orgID, mac,
	).Scan(&u.ID, &u.LocationID, &u.Name, &u.Kind, &u.MinTempF, &u.MaxTempF, &u.LogIntervalMinutes, &u.SensorMAC)
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, ErrNotFound
	}
	return u, err
}

func (s *Store) SetUnitSensorMAC(ctx context.Context, orgID, unitID uuid.UUID, mac string) (Unit, error) {
	var u Unit
	err := s.pool.QueryRow(ctx,
		`UPDATE units SET sensor_mac = $3 WHERE id = $1 AND org_id = $2
		 RETURNING id, location_id, name, kind, min_temp_f, max_temp_f, log_interval_minutes, sensor_mac`,
		unitID, orgID, nullify(mac),
	).Scan(&u.ID, &u.LocationID, &u.Name, &u.Kind, &u.MinTempF, &u.MaxTempF, &u.LogIntervalMinutes, &u.SensorMAC)
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, ErrNotFound
	}
	return u, err
}

// CreateSensorReading inserts a reading from a gateway, preserving the timestamp
// the gateway recorded (which may be older than now if it was buffered offline).
func (s *Store) CreateSensorReading(ctx context.Context, orgID, unitID uuid.UUID, tempF float64, recordedAt time.Time) error {
	at := recordedAt
	_, err := s.insertChainedReading(ctx, orgID, unitID, tempF, "sensor", "", nil, &at)
	return err
}

// ---------- alerting ----------

// ComputeStatus is the single source of truth for a unit's board/alert status.
// Used by both the dashboard endpoint and the alert engine.
func ComputeStatus(minF, maxF float64, intervalMin int, latestTempF *float64, latestAt *time.Time, now time.Time) string {
	if latestTempF == nil || latestAt == nil {
		return "no_data"
	}
	t := *latestTempF
	if t < minF || t > maxF {
		return "out_of_range"
	}
	if now.Sub(*latestAt) > time.Duration(intervalMin)*time.Minute {
		return "overdue"
	}
	return "ok"
}

type UnitEval struct {
	OrgID              uuid.UUID
	UnitID             uuid.UUID
	LocationID         uuid.UUID
	UnitName           string
	LocationName       string
	MinTempF           float64
	MaxTempF           float64
	LogIntervalMinutes int
	LatestTempF        *float64
	LatestAt           *time.Time
}

// EvaluateUnits returns every unit across all orgs with its latest reading, for
// the alert engine to assess. Fine at MVP scale; revisit if unit counts get large.
func (s *Store) EvaluateUnits(ctx context.Context) ([]UnitEval, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT u.org_id, u.id, u.location_id, u.name, l.name,
		        u.min_temp_f, u.max_temp_f, u.log_interval_minutes,
		        lr.temp_f, lr.recorded_at
		 FROM units u
		 JOIN locations l ON l.id = u.location_id
		 LEFT JOIN LATERAL (
		     SELECT temp_f, recorded_at FROM readings r
		     WHERE r.unit_id = u.id ORDER BY recorded_at DESC LIMIT 1
		 ) lr ON true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []UnitEval{}
	for rows.Next() {
		var e UnitEval
		if err := rows.Scan(&e.OrgID, &e.UnitID, &e.LocationID, &e.UnitName, &e.LocationName,
			&e.MinTempF, &e.MaxTempF, &e.LogIntervalMinutes, &e.LatestTempF, &e.LatestAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type Alert struct {
	ID         uuid.UUID  `json:"id"`
	OrgID      uuid.UUID  `json:"org_id"`
	UnitID     uuid.UUID  `json:"unit_id"`
	UnitName   string     `json:"unit_name"`
	Kind       string     `json:"kind"`
	Status     string     `json:"status"`
	Detail     string     `json:"detail"`
	OpenedAt   time.Time  `json:"opened_at"`
	NotifiedAt *time.Time `json:"notified_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
	// CorrectiveActionCount is populated by listings that report on documentation
	// status; it is 0 in engine-internal queries.
	CorrectiveActionCount int `json:"corrective_action_count"`
}

func (s *Store) ListOpenAlertsForUnit(ctx context.Context, unitID uuid.UUID) ([]Alert, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, org_id, unit_id, kind, status, COALESCE(detail,''), opened_at, notified_at, resolved_at
		 FROM alerts WHERE unit_id = $1 AND status = 'open'`, unitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alert{}
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.OrgID, &a.UnitID, &a.Kind, &a.Status, &a.Detail, &a.OpenedAt, &a.NotifiedAt, &a.ResolvedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) CreateAlert(ctx context.Context, orgID, unitID uuid.UUID, kind, detail string) (Alert, error) {
	var a Alert
	err := s.pool.QueryRow(ctx,
		`INSERT INTO alerts (org_id, unit_id, kind, detail) VALUES ($1,$2,$3,$4)
		 RETURNING id, org_id, unit_id, kind, status, COALESCE(detail,''), opened_at, notified_at, resolved_at`,
		orgID, unitID, kind, nullify(detail),
	).Scan(&a.ID, &a.OrgID, &a.UnitID, &a.Kind, &a.Status, &a.Detail, &a.OpenedAt, &a.NotifiedAt, &a.ResolvedAt)
	return a, err
}

func (s *Store) ResolveAlert(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE alerts SET status='resolved', resolved_at=now() WHERE id=$1`, id)
	return err
}

func (s *Store) MarkAlertNotified(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE alerts SET notified_at=now() WHERE id=$1`, id)
	return err
}

// AdminEmailsByOrg returns the email addresses alerts should go to.
func (s *Store) AdminEmailsByOrg(ctx context.Context, orgID uuid.UUID) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT email FROM users WHERE org_id=$1 AND role='admin'`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) ListRecentAlerts(ctx context.Context, orgID, locationID uuid.UUID, limit int) ([]Alert, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT a.id, a.org_id, a.unit_id, u.name, a.kind, a.status, COALESCE(a.detail,''),
		        a.opened_at, a.notified_at, a.resolved_at,
		        (SELECT count(*) FROM corrective_actions ca WHERE ca.alert_id = a.id)
		 FROM alerts a JOIN units u ON u.id = a.unit_id
		 WHERE a.org_id = $1 AND u.location_id = $2
		 ORDER BY a.opened_at DESC LIMIT $3`,
		orgID, locationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alert{}
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.OrgID, &a.UnitID, &a.UnitName, &a.Kind, &a.Status, &a.Detail, &a.OpenedAt, &a.NotifiedAt, &a.ResolvedAt, &a.CorrectiveActionCount); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---------- team invites & users ----------

type Invite struct {
	ID         uuid.UUID  `json:"id"`
	OrgID      uuid.UUID  `json:"org_id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (s *Store) CreateInvite(ctx context.Context, orgID uuid.UUID, email, role, tokenHash string, invitedBy uuid.UUID, expiresAt time.Time) (Invite, error) {
	var inv Invite
	err := s.pool.QueryRow(ctx,
		`INSERT INTO invites (org_id, email, role, token_hash, invited_by, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING id, org_id, email, role, expires_at, accepted_at, created_at`,
		orgID, email, role, tokenHash, invitedBy, expiresAt,
	).Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt)
	return inv, err
}

// InviteByTokenHash returns a pending (unaccepted, unexpired) invite.
func (s *Store) InviteByTokenHash(ctx context.Context, tokenHash string) (Invite, error) {
	var inv Invite
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, email, role, expires_at, accepted_at, created_at
		 FROM invites WHERE token_hash = $1 AND accepted_at IS NULL AND expires_at > now()`,
		tokenHash,
	).Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	return inv, err
}

func (s *Store) ListInvites(ctx context.Context, orgID uuid.UUID) ([]Invite, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, org_id, email, role, expires_at, accepted_at, created_at
		 FROM invites WHERE org_id = $1 AND accepted_at IS NULL AND expires_at > now()
		 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Invite{}
	for rows.Next() {
		var inv Invite
		if err := rows.Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (s *Store) DeleteInvite(ctx context.Context, orgID, inviteID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM invites WHERE id = $1 AND org_id = $2`, inviteID, orgID)
	return err
}

// AcceptInvite creates the user and marks the invite accepted, in one tx.
func (s *Store) AcceptInvite(ctx context.Context, inviteID, orgID uuid.UUID, email, name, role, hash string) (User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)

	var u User
	if err := tx.QueryRow(ctx,
		`INSERT INTO users (org_id, email, name, password_hash, role)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id, org_id, email, name, role`,
		orgID, email, name, hash, role,
	).Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.Role); err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE invites SET accepted_at = now() WHERE id = $1`, inviteID); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return u, nil
}

func (s *Store) ListUsers(ctx context.Context, orgID uuid.UUID) ([]User, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, org_id, email, name, role FROM users WHERE org_id = $1 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.Role); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ---------- password reset ----------

func (s *Store) CreatePasswordReset(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO password_resets (user_id, token_hash, expires_at) VALUES ($1,$2,$3)`,
		userID, tokenHash, expiresAt)
	return err
}

// PasswordResetUserByToken returns the reset id and user id for a pending token.
func (s *Store) PasswordResetUserByToken(ctx context.Context, tokenHash string) (resetID, userID uuid.UUID, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT id, user_id FROM password_resets
		 WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()`, tokenHash,
	).Scan(&resetID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, ErrNotFound
	}
	return resetID, userID, err
}

// ResetPassword updates the password and marks the token used, in one tx.
func (s *Store) ResetPassword(ctx context.Context, resetID, userID uuid.UUID, hash string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, hash); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE password_resets SET used_at = now() WHERE id = $1`, resetID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ---------- corrective actions ----------

type CorrectiveAction struct {
	ID             uuid.UUID  `json:"id"`
	OrgID          uuid.UUID  `json:"org_id"`
	AlertID        uuid.UUID  `json:"alert_id"`
	Action         string     `json:"action"`
	Disposition    string     `json:"disposition"`
	Note           string     `json:"note"`
	RecordedBy     *uuid.UUID `json:"recorded_by"`
	RecordedByName string     `json:"recorded_by_name"`
	CreatedAt      time.Time  `json:"created_at"`
}

// AlertByID returns a single org-scoped alert (used to validate before attaching
// a corrective action).
func (s *Store) AlertByID(ctx context.Context, orgID, id uuid.UUID) (Alert, error) {
	var a Alert
	err := s.pool.QueryRow(ctx,
		`SELECT a.id, a.org_id, a.unit_id, u.name, a.kind, a.status, COALESCE(a.detail,''),
		        a.opened_at, a.notified_at, a.resolved_at
		 FROM alerts a JOIN units u ON u.id = a.unit_id
		 WHERE a.id = $1 AND a.org_id = $2`, id, orgID,
	).Scan(&a.ID, &a.OrgID, &a.UnitID, &a.UnitName, &a.Kind, &a.Status, &a.Detail, &a.OpenedAt, &a.NotifiedAt, &a.ResolvedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Alert{}, ErrNotFound
	}
	return a, err
}

func (s *Store) CreateCorrectiveAction(ctx context.Context, orgID, alertID uuid.UUID, action, disposition, note string, recordedBy uuid.UUID) (CorrectiveAction, error) {
	ca := CorrectiveAction{
		OrgID: orgID, AlertID: alertID, Action: action,
		Disposition: disposition, Note: note,
	}
	rb := recordedBy
	ca.RecordedBy = &rb
	err := s.pool.QueryRow(ctx,
		`INSERT INTO corrective_actions (org_id, alert_id, action, disposition, note, recorded_by)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, created_at`,
		orgID, alertID, action, disposition, note, recordedBy,
	).Scan(&ca.ID, &ca.CreatedAt)
	return ca, err
}

func (s *Store) ListCorrectiveActionsForAlert(ctx context.Context, orgID, alertID uuid.UUID) ([]CorrectiveAction, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT ca.id, ca.org_id, ca.alert_id, ca.action, ca.disposition, ca.note,
		        ca.recorded_by, COALESCE(usr.name,'(removed)'), ca.created_at
		 FROM corrective_actions ca LEFT JOIN users usr ON usr.id = ca.recorded_by
		 WHERE ca.org_id = $1 AND ca.alert_id = $2
		 ORDER BY ca.created_at`, orgID, alertID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCorrectiveActions(rows)
}

// AlertsForLocationWindow returns alerts that opened within [from,to) for a
// location, oldest-first, for the compliance report.
func (s *Store) AlertsForLocationWindow(ctx context.Context, orgID, locationID uuid.UUID, from, to time.Time) ([]Alert, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT a.id, a.org_id, a.unit_id, u.name, a.kind, a.status, COALESCE(a.detail,''),
		        a.opened_at, a.notified_at, a.resolved_at
		 FROM alerts a JOIN units u ON u.id = a.unit_id
		 WHERE a.org_id = $1 AND u.location_id = $2 AND a.opened_at >= $3 AND a.opened_at < $4
		 ORDER BY a.opened_at`, orgID, locationID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alert{}
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.OrgID, &a.UnitID, &a.UnitName, &a.Kind, &a.Status, &a.Detail, &a.OpenedAt, &a.NotifiedAt, &a.ResolvedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CorrectiveActionsForLocationWindow returns corrective actions tied to alerts
// that opened within [from,to) for a location, for the compliance report.
func (s *Store) CorrectiveActionsForLocationWindow(ctx context.Context, orgID, locationID uuid.UUID, from, to time.Time) ([]CorrectiveAction, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT ca.id, ca.org_id, ca.alert_id, ca.action, ca.disposition, ca.note,
		        ca.recorded_by, COALESCE(usr.name,'(removed)'), ca.created_at
		 FROM corrective_actions ca
		 JOIN alerts a ON a.id = ca.alert_id
		 JOIN units u ON u.id = a.unit_id
		 LEFT JOIN users usr ON usr.id = ca.recorded_by
		 WHERE ca.org_id = $1 AND u.location_id = $2 AND a.opened_at >= $3 AND a.opened_at < $4
		 ORDER BY ca.created_at`, orgID, locationID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCorrectiveActions(rows)
}

func scanCorrectiveActions(rows pgx.Rows) ([]CorrectiveAction, error) {
	out := []CorrectiveAction{}
	for rows.Next() {
		var ca CorrectiveAction
		if err := rows.Scan(&ca.ID, &ca.OrgID, &ca.AlertID, &ca.Action, &ca.Disposition, &ca.Note,
			&ca.RecordedBy, &ca.RecordedByName, &ca.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ca)
	}
	return out, rows.Err()
}
