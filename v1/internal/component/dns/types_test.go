package dns

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewComponent(t *testing.T) {
	comp := NewComponent()
	assert.NotNil(t, comp)
	assert.Equal(t, "dns", comp.Name())
}

func TestComponentName(t *testing.T) {
	comp := NewComponent()
	assert.Equal(t, "dns", comp.Name())
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, "49", cfg.ImageTag)
	assert.Equal(t, "gsqlite3", cfg.Backend)
	assert.Equal(t, []string{"8.8.8.8", "1.1.1.1"}, cfg.Forwarders)
	assert.Equal(t, "/var/lib/powerdns", cfg.DataDir)
	assert.Equal(t, "/etc/powerdns", cfg.ConfigDir)
}

func TestConfigCustomization(t *testing.T) {
	cfg := DefaultConfig()

	// Test that we can customize the config
	cfg.ImageTag = "50"
	cfg.Backend = "postgresql"
	cfg.Forwarders = []string{"1.1.1.1"}
	cfg.DataDir = "/custom/data"
	cfg.ConfigDir = "/custom/config"

	assert.Equal(t, "50", cfg.ImageTag)
	assert.Equal(t, "postgresql", cfg.Backend)
	assert.Equal(t, []string{"1.1.1.1"}, cfg.Forwarders)
	assert.Equal(t, "/custom/data", cfg.DataDir)
	assert.Equal(t, "/custom/config", cfg.ConfigDir)
}

func TestZoneStruct(t *testing.T) {
	zone := Zone{
		ID:     "1",
		Name:   "example.com",
		Type:   "NATIVE",
		Serial: 1,
	}

	assert.Equal(t, "1", zone.ID)
	assert.Equal(t, "example.com", zone.Name)
	assert.Equal(t, "NATIVE", zone.Type)
	assert.Equal(t, uint32(1), zone.Serial)
}

func TestRecordStruct(t *testing.T) {
	record := Record{
		Name:     "test.example.com",
		Type:     "A",
		Content:  "192.168.1.10",
		TTL:      3600,
		Disabled: false,
	}

	assert.Equal(t, "test.example.com", record.Name)
	assert.Equal(t, "A", record.Type)
	assert.Equal(t, "192.168.1.10", record.Content)
	assert.Equal(t, 3600, record.TTL)
	assert.False(t, record.Disabled)
}

func TestRecordSetStruct(t *testing.T) {
	recordSet := RecordSet{
		Name: "test.example.com",
		Type: "A",
		TTL:  3600,
		Records: []Record{
			{
				Name:    "test.example.com",
				Type:    "A",
				Content: "192.168.1.10",
			},
			{
				Name:    "test.example.com",
				Type:    "A",
				Content: "192.168.1.11",
			},
		},
	}

	assert.Equal(t, "test.example.com", recordSet.Name)
	assert.Equal(t, "A", recordSet.Type)
	assert.Equal(t, 3600, recordSet.TTL)
	assert.Len(t, recordSet.Records, 2)
}
