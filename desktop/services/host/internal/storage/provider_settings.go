package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

func (s *Store) ProviderSettings(ctx context.Context) (model.CLISettings, error) {
	out := model.CLISettings{Version: 1, Providers: []model.CLIProviderSetting{}}
	var body []byte
	err := s.db.QueryRowContext(ctx, "SELECT body FROM provider_settings WHERE id='cli'").Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(body, &out); err != nil {
		return out, fmt.Errorf("应用 CLI 设置损坏：%w", err)
	}
	if out.Version > 1 {
		return out, fmt.Errorf("应用 CLI 设置版本 %d 高于当前支持版本", out.Version)
	}
	// version=0 是早期无版本数据，字段仍可无损读取。
	out.Version = 1
	if out.Providers == nil {
		out.Providers = []model.CLIProviderSetting{}
	}
	return out, nil
}
func (s *Store) SaveProviderSettings(ctx context.Context, in model.CLISettings) error {
	in.Version = 1
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO provider_settings(id,body) VALUES('cli',?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", body)
	return err
}
func (s *Store) ResetProviderSettings(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM provider_settings WHERE id='cli'")
	return err
}
func (s *Store) ProviderCatalog(ctx context.Context, id string) ([]byte, error) {
	var body []byte
	err := s.db.QueryRowContext(ctx, "SELECT body FROM provider_settings WHERE id=?", "catalog:"+id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return body, err
}
func (s *Store) SaveProviderCatalog(ctx context.Context, id string, body []byte) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO provider_settings(id,body) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", "catalog:"+id, body)
	return err
}
