package repository

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"sensor-backend/Src/internal/model"
	"sensor-backend/Src/pkg/utils"

	"go.uber.org/zap"
)

// Storage 数据存储结构
type Storage struct {
	mu         sync.RWMutex
	dataDir    string
	realtime   map[string]*model.SensorData // 实时数据缓存
	deviceSeen map[string]time.Time          // 设备最后活跃时间
}

// NewStorage 创建存储实例
func NewStorage(dataDir string) *Storage {
	// 确保目录存在
	os.MkdirAll(dataDir, 0755)
	return &Storage{
		dataDir:    dataDir,
		realtime:   make(map[string]*model.SensorData),
		deviceSeen: make(map[string]time.Time),
	}
}

// UpdateRealtime 更新实时数据（合并同批次数据）
func (s *Storage) UpdateRealtime(data *model.SensorData) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.realtime[data.DeviceID]
	if exists && existing.Timestamp == data.Timestamp {
		// 相同时间戳，合并数据
		existing.Data = mergeData(existing.Data, data.Data)
	} else {
		// 新数据，直接替换
		newData := *data
		s.realtime[data.DeviceID] = &newData
	}
	s.deviceSeen[data.DeviceID] = time.Now()
}

// GetRealtime 获取实时数据
func (s *Storage) GetRealtime(deviceID string) *model.SensorData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if data, ok := s.realtime[deviceID]; ok {
		// 返回副本避免外部修改
		copy := *data
		return &copy
	}
	return nil
}

// GetAllRealtime 获取所有实时数据
func (s *Storage) GetAllRealtime() map[string]*model.SensorData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*model.SensorData)
	for k, v := range s.realtime {
		copy := *v
		result[k] = &copy
	}
	return result
}

// IsDeviceOnline 检查设备是否在线（60秒内有数据）
func (s *Storage) IsDeviceOnline(deviceID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if lastSeen, ok := s.deviceSeen[deviceID]; ok {
		return time.Since(lastSeen) < 60*time.Second
	}
	return false
}

// GetDeviceLastSeen 获取设备最后活跃时间
func (s *Storage) GetDeviceLastSeen(deviceID string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if lastSeen, ok := s.deviceSeen[deviceID]; ok {
		return lastSeen
	}
	return time.Time{}
}

// SaveHistory 保存历史数据
func (s *Storage) SaveHistory(data *model.SensorData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	date := time.Unix(data.Timestamp, 0).Format("2006-01-02")
	filePath := filepath.Join(s.dataDir, date+".json")

	// 读取现有数据
	var historyFile model.HistoryFile
	if content, err := os.ReadFile(filePath); err == nil {
		json.Unmarshal(content, &historyFile)
	}

	// 检查是否已存在相同时间戳的记录（合并同批次数据）
	existingIdx := -1
	for i, record := range historyFile.Records {
		if record.Timestamp == data.Timestamp && record.DeviceID == data.DeviceID {
			existingIdx = i
			break
		}
	}

	// 合并数据：保留已有数据，同时补充新数据中的非零值
	newRecord := model.HistoryRecord{
		Timestamp: data.Timestamp,
		DeviceID:  data.DeviceID,
	}

	if existingIdx >= 0 {
		// 合并到已有记录
		existing := historyFile.Records[existingIdx]
		newRecord.Data = mergeData(existing.Data, data.Data)
		historyFile.Records[existingIdx] = newRecord
	} else {
		// 添加新记录
		newRecord.Data = data.Data
		historyFile.Records = append(historyFile.Records, newRecord)
	}

	historyFile.Date = date

	// 原子写入：先写临时文件，再重命名
	tmpPath := filePath + ".tmp"
	content, err := json.MarshalIndent(historyFile, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(tmpPath, content, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, filePath)
}

// mergeData 合并两条传感器数据，保留非零值
func mergeData(existing, new model.Data) model.Data {
	result := existing
	if new.Temperature != 0 {
		result.Temperature = new.Temperature
	}
	if new.Humidity != 0 {
		result.Humidity = new.Humidity
	}
	if new.Pressure != 0 {
		result.Pressure = new.Pressure
	}
	if new.Light != 0 {
		result.Light = new.Light
	}
	if new.AirQuality != 0 {
		result.AirQuality = new.AirQuality
	}
	// motion 和 rain 保留最新值
	if new.Motion != existing.Motion {
		result.Motion = new.Motion
	}
	if new.Rain != existing.Rain {
		result.Rain = new.Rain
	}
	return result
}

// QueryHistory 查询历史数据
func (s *Storage) QueryHistory(deviceID string, startTime, endTime int64, page, pageSize int) ([]model.HistoryRecord, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 计算日期范围
	startDate := time.Unix(startTime, 0)
	endDate := time.Unix(endTime, 0)

	var allRecords []model.HistoryRecord

	// 遍历日期范围内的所有文件
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		filePath := filepath.Join(s.dataDir, dateStr+".json")

		var historyFile model.HistoryFile
		if content, err := os.ReadFile(filePath); err == nil {
			json.Unmarshal(content, &historyFile)
			for _, record := range historyFile.Records {
				if record.Timestamp >= startTime && record.Timestamp <= endTime {
					if deviceID == "" || record.DeviceID == deviceID {
						allRecords = append(allRecords, record)
					}
				}
			}
		}
	}

	// 按时间倒序排序
	sort.Slice(allRecords, func(i, j int) bool {
		return allRecords[i].Timestamp > allRecords[j].Timestamp
	})

	total := len(allRecords)

	// 分页
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 2000 {
		pageSize = 2000  // 提高最大限制以支持更长时间范围的数据查询
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		return []model.HistoryRecord{}, total, nil
	}
	if end > total {
		end = total
	}

	return allRecords[start:end], total, nil
}

// GetTotalRecords 获取设备总记录数
func (s *Storage) GetTotalRecords(deviceID string) int {
	count := 0
	files, _ := filepath.Glob(filepath.Join(s.dataDir, "*.json"))
	for _, file := range files {
		var historyFile model.HistoryFile
		if content, err := os.ReadFile(file); err == nil {
			json.Unmarshal(content, &historyFile)
			for _, record := range historyFile.Records {
				if deviceID == "" || record.DeviceID == deviceID {
					count++
				}
			}
		}
	}
	return count
}

// CleanupOldData 清理过期数据
func (s *Storage) CleanupOldData(retentionDays int) error {
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)
	files, err := filepath.Glob(filepath.Join(s.dataDir, "*.json"))
	if err != nil {
		return err
	}

	deleted := 0
	for _, file := range files {
		filename := filepath.Base(file)
		dateStr := filename[:len(filename)-5] // 去掉.json
		if date, err := time.Parse("2006-01-02", dateStr); err == nil {
			if date.Before(cutoffDate) {
				if err := os.Remove(file); err == nil {
					deleted++
					utils.Logger.Info("删除过期历史文件", zap.String("file", filename))
				}
			}
		}
	}

	return nil
}

// GetDataDir 获取数据目录
func (s *Storage) GetDataDir() string {
	return s.dataDir
}
