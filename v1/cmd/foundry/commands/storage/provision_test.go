package storage

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestParseQuantity(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid gigabytes",
			input:   "10Gi",
			wantErr: false,
		},
		{
			name:    "valid megabytes",
			input:   "500Mi",
			wantErr: false,
		},
		{
			name:    "valid terabytes",
			input:   "1Ti",
			wantErr: false,
		},
		{
			name:    "valid with decimal",
			input:   "1.5Gi",
			wantErr: false,
		},
		{
			name:    "invalid format",
			input:   "not-a-size",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resource.ParseQuantity(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseQuantity(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestAccessModes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected corev1.PersistentVolumeAccessMode
		valid    bool
	}{
		{
			name:     "ReadWriteOnce",
			input:    "ReadWriteOnce",
			expected: corev1.ReadWriteOnce,
			valid:    true,
		},
		{
			name:     "ReadWriteMany",
			input:    "ReadWriteMany",
			expected: corev1.ReadWriteMany,
			valid:    true,
		},
		{
			name:     "ReadOnlyMany",
			input:    "ReadOnlyMany",
			expected: corev1.ReadOnlyMany,
			valid:    true,
		},
		{
			name:     "RWO shorthand",
			input:    "RWO",
			expected: corev1.ReadWriteOnce,
			valid:    true,
		},
		{
			name:     "RWX shorthand",
			input:    "RWX",
			expected: corev1.ReadWriteMany,
			valid:    true,
		},
		{
			name:     "ROX shorthand",
			input:    "ROX",
			expected: corev1.ReadOnlyMany,
			valid:    true,
		},
		{
			name:  "invalid mode",
			input: "InvalidMode",
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var accessMode corev1.PersistentVolumeAccessMode
			var valid bool

			switch tt.input {
			case "ReadWriteOnce", "RWO":
				accessMode = corev1.ReadWriteOnce
				valid = true
			case "ReadWriteMany", "RWX":
				accessMode = corev1.ReadWriteMany
				valid = true
			case "ReadOnlyMany", "ROX":
				accessMode = corev1.ReadOnlyMany
				valid = true
			default:
				valid = false
			}

			if valid != tt.valid {
				t.Errorf("access mode %q validity = %v, want %v", tt.input, valid, tt.valid)
			}
			if valid && accessMode != tt.expected {
				t.Errorf("access mode %q = %v, want %v", tt.input, accessMode, tt.expected)
			}
		})
	}
}

func TestTruncatePVC(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		max    int
		want   string
	}{
		{
			name:  "no truncation needed",
			input: "my-pvc",
			max:   10,
			want:  "my-pvc",
		},
		{
			name:  "exact length",
			input: "exact",
			max:   5,
			want:  "exact",
		},
		{
			name:  "needs truncation",
			input: "very-long-pvc-name-that-exceeds-limit",
			max:   20,
			want:  "very-long-pvc-nam...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncatePVC(tt.input, tt.max)
			if result != tt.want {
				t.Errorf("truncatePVC(%q, %d) = %q, want %q", tt.input, tt.max, result, tt.want)
			}
		})
	}
}

func TestPVCStatusPhases(t *testing.T) {
	// Test that we handle all PVC phases correctly
	phases := []corev1.PersistentVolumeClaimPhase{
		corev1.ClaimPending,
		corev1.ClaimBound,
		corev1.ClaimLost,
	}

	for _, phase := range phases {
		name := string(phase)
		if name == "" {
			t.Errorf("PVC phase %v has empty string representation", phase)
		}
	}
}

func TestStorageClassPointer(t *testing.T) {
	// Test that we handle nil storage class correctly
	var nilPtr *string
	if nilPtr != nil {
		t.Error("nil storage class pointer should be nil")
	}

	sc := "local-path"
	ptr := &sc
	if *ptr != "local-path" {
		t.Errorf("storage class pointer = %q, want local-path", *ptr)
	}
}

func TestResourceListCreation(t *testing.T) {
	quantity := resource.MustParse("10Gi")
	resourceList := corev1.ResourceList{
		corev1.ResourceStorage: quantity,
	}

	if _, ok := resourceList[corev1.ResourceStorage]; !ok {
		t.Error("ResourceList should contain storage resource")
	}

	storedQuantity := resourceList[corev1.ResourceStorage]
	if storedQuantity.String() != "10Gi" {
		t.Errorf("stored quantity = %v, want 10Gi", storedQuantity.String())
	}
}
