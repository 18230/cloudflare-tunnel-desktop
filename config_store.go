package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const appConfigDirName = "CloudflareTunnelDesktop"

// ConfigStore 负责读写本地配置文件，文件权限限制为当前用户可读写。
type ConfigStore struct {
	path string
}

// NewConfigStore 创建使用系统配置目录的配置存储。
func NewConfigStore() (*ConfigStore, error) {
	baseDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("获取系统配置目录失败: %w", err)
	}
	return &ConfigStore{path: filepath.Join(baseDir, appConfigDirName, "config.json")}, nil
}

// NewConfigStoreAt 创建指定路径的配置存储，主要用于测试。
func NewConfigStoreAt(path string) *ConfigStore {
	return &ConfigStore{path: path}
}

// Load 读取配置文件，不存在时返回默认配置。
func (s *ConfigStore) Load() (AppConfig, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return NewDefaultConfig(), nil
	}
	if err != nil {
		return AppConfig{}, fmt.Errorf("读取配置失败: %w", err)
	}
	config := NewDefaultConfig()
	if err := json.Unmarshal(data, &config); err != nil {
		return AppConfig{}, fmt.Errorf("解析配置失败: %w", err)
	}
	config.Protocol = normalizeProtocol(config.Protocol)
	config.AuthType = NormalizeAuthType(config.AuthType)
	if config.Routes == nil {
		config.Routes = []Route{}
	}
	return config, nil
}

// Save 校验并写入本地配置，API Token 和 Tunnel Token 按用户要求明文保存。
func (s *ConfigStore) Save(config AppConfig) error {
	config.Protocol = normalizeProtocol(config.Protocol)
	config.AuthType = NormalizeAuthType(config.AuthType)
	if config.Routes == nil {
		config.Routes = []Route{}
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	return nil
}

// Path 返回配置文件路径，方便 UI 和测试展示。
func (s *ConfigStore) Path() string {
	return s.path
}
