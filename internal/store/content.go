package store

import "database/sql"

type Group struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Sort int    `json:"sort"`
}

type Service struct {
	ID          int64   `json:"id"`
	GroupID     int64   `json:"groupId"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Icon        string  `json:"icon"`
	Color       string  `json:"color"`
	IconFit     string  `json:"iconFit"` // "contain" (centred on a colour tile) | "cover" (fills the tile)
	LanURL      string  `json:"lanUrl"`
	WanURL      string  `json:"wanUrl"`
	NewTab      bool    `json:"newTab"`
	HealthCheck bool    `json:"healthCheck"`
	Sort        int     `json:"sort"`
	Teams       []int64 `json:"teams"`
}

type Team struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ---- groups ----

func (s *Store) ListGroups() ([]Group, error) {
	rows, err := s.db.Query(`SELECT id,name,sort FROM groups ORDER BY sort,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Sort); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) CreateGroup(name string) (*Group, error) {
	res, err := s.db.Exec(`INSERT INTO groups(name,sort) VALUES(?,COALESCE((SELECT MAX(sort)+1 FROM groups),0))`, name)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	g := &Group{ID: id, Name: name}
	_ = s.db.QueryRow(`SELECT sort FROM groups WHERE id=?`, id).Scan(&g.Sort)
	return g, nil
}

func (s *Store) RenameGroup(id int64, name string) error {
	res, err := s.db.Exec(`UPDATE groups SET name=? WHERE id=?`, name, id)
	return affected(res, err)
}

func (s *Store) DeleteGroup(id int64) error {
	res, err := s.db.Exec(`DELETE FROM groups WHERE id=?`, id)
	return affected(res, err)
}

func (s *Store) ReorderGroups(ids []int64) error {
	return s.reorder(`UPDATE groups SET sort=? WHERE id=?`, ids)
}

func (s *Store) reorder(q string, ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.Exec(q, i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- services ----

const svcCols = `id,group_id,name,description,icon,color,icon_fit,lan_url,wan_url,new_tab,health_check,sort`

func scanService(r interface{ Scan(...any) error }) (Service, error) {
	var v Service
	var nt, hc int
	err := r.Scan(&v.ID, &v.GroupID, &v.Name, &v.Description, &v.Icon, &v.Color, &v.IconFit, &v.LanURL, &v.WanURL, &nt, &hc, &v.Sort)
	v.NewTab, v.HealthCheck, v.Teams = nt == 1, hc == 1, []int64{}
	return v, err
}

func (s *Store) ListServices() ([]Service, error) {
	rows, err := s.db.Query(`SELECT ` + svcCols + ` FROM services ORDER BY sort,id`)
	if err != nil {
		return nil, err
	}
	out := []Service{}
	idx := map[int64]int{}
	for rows.Next() {
		v, err := scanService(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		idx[v.ID] = len(out)
		out = append(out, v)
	}
	rows.Close()
	tr, err := s.db.Query(`SELECT service_id,team_id FROM service_teams`)
	if err != nil {
		return nil, err
	}
	defer tr.Close()
	for tr.Next() {
		var sid, tid int64
		if tr.Scan(&sid, &tid) == nil {
			if i, ok := idx[sid]; ok {
				out[i].Teams = append(out[i].Teams, tid)
			}
		}
	}
	return out, nil
}

func (s *Store) GetService(id int64) (*Service, error) {
	list, err := s.ListServices()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) SaveService(v *Service) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if v.ID == 0 {
		res, err := tx.Exec(`INSERT INTO services(group_id,name,description,icon,color,icon_fit,lan_url,wan_url,new_tab,health_check,sort)
			VALUES(?,?,?,?,?,?,?,?,?,?,COALESCE((SELECT MAX(sort)+1 FROM services WHERE group_id=?),0))`,
			v.GroupID, v.Name, v.Description, v.Icon, v.Color, v.IconFit, v.LanURL, v.WanURL, b2i(v.NewTab), b2i(v.HealthCheck), v.GroupID)
		if err != nil {
			return err
		}
		v.ID, _ = res.LastInsertId()
	} else {
		res, err := tx.Exec(`UPDATE services SET group_id=?,name=?,description=?,icon=?,color=?,icon_fit=?,lan_url=?,wan_url=?,new_tab=?,health_check=? WHERE id=?`,
			v.GroupID, v.Name, v.Description, v.Icon, v.Color, v.IconFit, v.LanURL, v.WanURL, b2i(v.NewTab), b2i(v.HealthCheck), v.ID)
		if err := affected(res, err); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM service_teams WHERE service_id=?`, v.ID); err != nil {
		return err
	}
	for _, t := range v.Teams {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO service_teams VALUES(?,?)`, v.ID, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteService(id int64) error {
	res, err := s.db.Exec(`DELETE FROM services WHERE id=?`, id)
	return affected(res, err)
}

// MoveServices sets group membership and order: layout maps group id -> ordered service ids.
func (s *Store) MoveServices(layout map[int64][]int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for gid, ids := range layout {
		for i, id := range ids {
			if _, err := tx.Exec(`UPDATE services SET group_id=?,sort=? WHERE id=?`, gid, i, id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// VisibleTo reports whether a service is visible to a user. A service with no
// teams is public to all logged-in users; admins see everything.
func VisibleTo(v *Service, u *User) bool {
	if u.Role == "admin" || len(v.Teams) == 0 {
		return true
	}
	for _, t := range v.Teams {
		for _, ut := range u.Teams {
			if t == ut {
				return true
			}
		}
	}
	return false
}

// ---- teams ----

func (s *Store) ListTeams() ([]Team, error) {
	rows, err := s.db.Query(`SELECT id,name FROM teams ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Team{}
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) CreateTeam(name string) (*Team, error) {
	res, err := s.db.Exec(`INSERT INTO teams(name) VALUES(?)`, name)
	if isUnique(err) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Team{ID: id, Name: name}, nil
}

// DeleteTeam refuses while a service is restricted to the team: the cascade
// would otherwise empty its ACL and silently make the service public.
func (s *Store) DeleteTeam(id int64) error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM service_teams WHERE team_id=?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrConflict
	}
	res, err := s.db.Exec(`DELETE FROM teams WHERE id=?`, id)
	return affected(res, err)
}
