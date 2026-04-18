package tsdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"sensor-backend/Src/internal/model"

	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// Client TimescaleDB 客户端
type Client struct {
	db     *sql.DB
	logger *zap.Logger
}

// Config TimescaleDB 配置
type Config struct {
	Host            string
	Port            int
	User            string
	Password        string
	DBName          string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// NewClient 创建 TimescaleDB 客户端
func NewClient(cfg Config, logger *zap.Logger) (*Client, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("数据库 Ping 失败: %w", err)
	}

	client := &Client{
		db:     db,
		logger: logger,
	}

	// 初始化表
	if err := client.initTables(); err != nil {
		return nil, fmt.Errorf("初始化表失败: %w", err)
	}

	logger.Info("TimescaleDB 连接成功")
	return client, nil
}

// initTables 初始化表结构
func (c *Client) initTables() error {
	// 创建时序 hypertable
	createHypertableQuery := `
	CREATE TABLE IF NOT EXISTS sensor_data (
		time        TIMESTAMPTZ NOT NULL,
		device_id   TEXT NOT NULL,
		temperature DOUBLE PRECISION,
		humidity    DOUBLE PRECISION,
		pressure    DOUBLE PRECISION,
		light       DOUBLE PRECISION,
		air_quality DOUBLE PRECISION,
		motion      INTEGER,
		rain        INTEGER
	);

	SELECT create_hypertable('sensor_data', 'time',
		if_not_exists => TRUE,
		migrate_data => TRUE
	);

	CREATE INDEX IF NOT EXISTS idx_sensor_data_device_id ON sensor_data (device_id);
	CREATE INDEX IF NOT EXISTS idx_sensor_data_time ON sensor_data (time DESC);
	`

	_, err := c.db.Exec(createHypertableQuery)
	if err != nil {
		return fmt.Errorf("创建时序表失败: %w", err)
	}

	// 创建设备状态表
	createDeviceTableQuery := `
	CREATE TABLE IF NOT EXISTS device_status (
		device_id   TEXT PRIMARY KEY,
		online      BOOLEAN DEFAULT FALSE,
		last_seen   TIMESTAMPTZ,
		total_records BIGINT DEFAULT 0,
		updated_at  TIMESTAMPTZ DEFAULT NOW()
	);
	`

	_, err = c.db.Exec(createDeviceTableQuery)
	if err != nil {
		return fmt.Errorf("创建设备状态表失败: %w", err)
	}

	c.logger.Info("TimescaleDB 表初始化完成")
	return nil
}

// InsertSensorData 插入传感器数据
func (c *Client) InsertSensorData(data *model.SensorData) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	insertQuery := `
	INSERT INTO sensor_data (time, device_id, temperature, humidity, pressure, light, air_quality, motion, rain)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := c.db.ExecContext(ctx, insertQuery,
		time.Unix(data.Timestamp, 0),
		data.DeviceID,
		data.Data.Temperature,
		data.Data.Humidity,
		data.Data.Pressure,
		data.Data.Light,
		data.Data.AirQuality,
		data.Data.Motion,
		data.Data.Rain,
	)

	if err != nil {
		return fmt.Errorf("插入传感器数据失败: %w", err)
	}

	// 更新设备状态
	c.updateDeviceStatus(data.DeviceID)
	return nil
}

// updateDeviceStatus 更新设备状态
func (c *Client) updateDeviceStatus(deviceID string) {
	updateQuery := `
	INSERT INTO device_status (device_id, online, last_seen, total_records, updated_at)
	VALUES ($1, TRUE, NOW(), 1, NOW())
	ON CONFLICT (device_id) DO UPDATE SET
		online = TRUE,
		last_seen = NOW(),
		total_records = device_status.total_records + 1,
		updated_at = NOW()
	`
	c.db.Exec(updateQuery, deviceID)
}

// GetLatestData 获取最新数据
func (c *Client) GetLatestData(deviceID string) (*model.SensorData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var query string
	var args []interface{}

	if deviceID == "" {
		query = `
		SELECT time, device_id, temperature, humidity, pressure, light, air_quality, motion, rain
		FROM sensor_data
		ORDER BY time DESC
		LIMIT 1
		`
	} else {
		query = `
		SELECT time, device_id, temperature, humidity, pressure, light, air_quality, motion, rain
		FROM sensor_data
		WHERE device_id = $1
		ORDER BY time DESC
		LIMIT 1
		`
		args = append(args, deviceID)
	}

	var timestamp time.Time
	var data model.SensorData

	err := c.db.QueryRowContext(ctx, query, args...).Scan(
		&timestamp,
		&data.DeviceID,
		&data.Data.Temperature,
		&data.Data.Humidity,
		&data.Data.Pressure,
		&data.Data.Light,
		&data.Data.AirQuality,
		&data.Data.Motion,
		&data.Data.Rain,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询最新数据失败: %w", err)
	}

	data.Timestamp = timestamp.Unix()
	return &data, nil
}

// GetAllLatestData 获取所有设备最新数据
func (c *Client) GetAllLatestData() (map[string]*model.SensorData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
	SELECT DISTINCT ON (device_id)
		time, device_id, temperature, humidity, pressure, light, air_quality, motion, rain
	FROM sensor_data
	ORDER BY device_id, time DESC
	`

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("查询所有设备最新数据失败: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*model.SensorData)
	for rows.Next() {
		var timestamp time.Time
		var data model.SensorData

		err := rows.Scan(
			&timestamp,
			&data.DeviceID,
			&data.Data.Temperature,
			&data.Data.Humidity,
			&data.Data.Pressure,
			&data.Data.Light,
			&data.Data.AirQuality,
			&data.Data.Motion,
			&data.Data.Rain,
		)
		if err != nil {
			continue
		}

		data.Timestamp = timestamp.Unix()
		result[data.DeviceID] = &data
	}

	return result, nil
}

// QueryHistory 查询历史数据
func (c *Client) QueryHistory(deviceID string, startTime, endTime int64, page, pageSize int) ([]model.HistoryRecord, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 先查询总数
	countQuery := `
	SELECT COUNT(*) FROM sensor_data
	WHERE time >= $1 AND time <= $2
	`
	args := []interface{}{time.Unix(startTime, 0), time.Unix(endTime, 0)}

	if deviceID != "" {
		countQuery += ` AND device_id = $3`
		args = append(args, deviceID)
	}

	var total int
	if err := c.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("查询总数失败: %w", err)
	}

	// 计算分页
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 2000 {
		pageSize = 2000
	}

	offset := (page - 1) * pageSize

	// 查询数据
	selectQuery := `
	SELECT time, device_id, temperature, humidity, pressure, light, air_quality, motion, rain
	FROM sensor_data
	WHERE time >= $1 AND time <= $2
	`
	selectArgs := []interface{}{time.Unix(startTime, 0), time.Unix(endTime, 0)}

	if deviceID != "" {
		selectQuery += ` AND device_id = $3`
		selectArgs = append(selectArgs, deviceID)
	}

	selectQuery += ` ORDER BY time DESC LIMIT $` + fmt.Sprintf("%d", len(selectArgs)+1) + ` OFFSET $` + fmt.Sprintf("%d", len(selectArgs)+2)
	selectArgs = append(selectArgs, pageSize, offset)

	rows, err := c.db.QueryContext(ctx, selectQuery, selectArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询历史数据失败: %w", err)
	}
	defer rows.Close()

	var records []model.HistoryRecord
	for rows.Next() {
		var timestamp time.Time
		var record model.HistoryRecord

		err := rows.Scan(
			&timestamp,
			&record.DeviceID,
			&record.Data.Temperature,
			&record.Data.Humidity,
			&record.Data.Pressure,
			&record.Data.Light,
			&record.Data.AirQuality,
			&record.Data.Motion,
			&record.Data.Rain,
		)
		if err != nil {
			continue
		}

		record.Timestamp = timestamp.Unix()
		records = append(records, record)
	}

	return records, total, nil
}

// GetDeviceStatus 获取设备状态
func (c *Client) GetDeviceStatus() ([]model.DeviceStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT device_id, online, last_seen, total_records FROM device_status`

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("查询设备状态失败: %w", err)
	}
	defer rows.Close()

	var statuses []model.DeviceStatus
	for rows.Next() {
		var status model.DeviceStatus
		if err := rows.Scan(&status.DeviceID, &status.Online, &status.LastSeen, &status.TotalRecords); err != nil {
			continue
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// IsDeviceOnline 检查设备是否在线
func (c *Client) IsDeviceOnline(deviceID string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var online bool
	var lastSeen time.Time

	query := `SELECT online, last_seen FROM device_status WHERE device_id = $1`
	err := c.db.QueryRowContext(ctx, query, deviceID).Scan(&online, &lastSeen)
	if err != nil {
		return false
	}

	// 如果标记在线但超过60秒没更新，标记为离线
	if online && time.Since(lastSeen) > 60*time.Second {
		c.updateDeviceOnlineStatus(deviceID, false)
		return false
	}

	return online
}

// updateDeviceOnlineStatus 更新设备在线状态
func (c *Client) updateDeviceOnlineStatus(deviceID string, online bool) {
	query := `UPDATE device_status SET online = $1, updated_at = NOW() WHERE device_id = $2`
	c.db.Exec(query, online, deviceID)
}

// GetTotalRecords 获取设备总记录数
func (c *Client) GetTotalRecords(deviceID string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var query string
	var args []interface{}

	if deviceID == "" {
		query = `SELECT COALESCE(SUM(total_records), 0) FROM device_status`
	} else {
		query = `SELECT COALESCE(total_records, 0) FROM device_status WHERE device_id = $1`
		args = append(args, deviceID)
	}

	var count int64
	if err := c.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("查询总记录数失败: %w", err)
	}

	return count, nil
}

// CleanupOldData 清理过期数据
func (c *Client) CleanupOldData(retentionDays int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)

	// 删除旧数据
	query := `DELETE FROM sensor_data WHERE time < $1`
	result, err := c.db.ExecContext(ctx, query, cutoffTime)
	if err != nil {
		return fmt.Errorf("清理历史数据失败: %w", err)
	}

	deleted, _ := result.RowsAffected()
	c.logger.Info("清理过期历史数据", zap.Int64("deleted_rows", deleted))

	return nil
}

// BatchInsert 批量插入数据
func (c *Client) BatchInsert(dataList []*model.SensorData) error {
	if len(dataList) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO sensor_data (time, device_id, temperature, humidity, pressure, light, air_quality, motion, rain)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`)
	if err != nil {
		return fmt.Errorf("准备语句失败: %w", err)
	}
	defer stmt.Close()

	for _, data := range dataList {
		_, err := stmt.ExecContext(ctx,
			time.Unix(data.Timestamp, 0),
			data.DeviceID,
			data.Data.Temperature,
			data.Data.Humidity,
			data.Data.Pressure,
			data.Data.Light,
			data.Data.AirQuality,
			data.Data.Motion,
			data.Data.Rain,
		)
		if err != nil {
			return fmt.Errorf("批量插入失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}

// Close 关闭连接
func (c *Client) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// Ping 检查连接
func (c *Client) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}
