package test

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/azure"
	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// You normally want to run this under a separate "Testing" subscription
// For lab purposes you will use your assigned subscription under the Cloud Dev/Ops program tenant
var subscriptionID string = "1f9a4d89-32f2-4e2f-abcf-e458452b32fa"

// Shared variables for all tests
var (
	terraformOptions  *terraform.Options
	vmName            string
	resourceGroupName string
	nicName           string
	once              sync.Once
	initialized       bool
)

// setupTerraform initializes Terraform and applies the configuration once
func setupTerraform(t *testing.T) {
	once.Do(func() {
		terraformOptions = &terraform.Options{
			TerraformDir: "../",
			Vars: map[string]interface{}{
				"labelPrefix": "lian0138",
			},
		}

		// Run `terraform init` and `terraform apply`
		terraform.InitAndApply(t, terraformOptions)

		// Retrieve outputs
		vmName = terraform.Output(t, terraformOptions, "vm_name")
		resourceGroupName = terraform.Output(t, terraformOptions, "resource_group_name")
		nicName = terraform.Output(t, terraformOptions, "nic_name")

		initialized = true
	})

	if !initialized {
		t.Fatal("Terraform setup failed")
	}
}

// cleanupTerraform destroys resources after all tests
func cleanupTerraform() {
	if initialized && terraformOptions != nil {
		terraform.Destroy(&testing.T{}, terraformOptions)
	}
}

func TestMain(m *testing.M) {
	// Create a timestamped log file
	timestamp := time.Now().Format("20060102_150405") // Format: YYYYMMDD_HHMMSS
	logFileName := fmt.Sprintf("test_%s.log", timestamp)
	logFile, err := os.Create(logFileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	// Redirect test output to the log file
	originalStdout := os.Stdout
	os.Stdout = logFile
	defer func() {
		os.Stdout = originalStdout
		// Cleanup resources after all tests
		cleanupTerraform()
		logFile.Sync()
	}()

	// Run tests and capture exit code
	exitCode := m.Run()

	// Exit with the test result code
	os.Exit(exitCode)
}

func TestAzureLinuxVMCreation(t *testing.T) {
	terraformOptions := &terraform.Options{
		// The path to where our Terraform code is located
		TerraformDir: "../",
		// Override the default terraform variables
		Vars: map[string]interface{}{
			"labelPrefix": "lian0138",
		},
	}

	defer terraform.Destroy(t, terraformOptions)

	// Run `terraform init` and `terraform apply`. Fail the test if there are any errors.
	terraform.InitAndApply(t, terraformOptions)

	// Run `terraform output` to get the value of output variable
	vmName := terraform.Output(t, terraformOptions, "vm_name")
	resourceGroupName := terraform.Output(t, terraformOptions, "resource_group_name")

	// Confirm VM exists
	assert.True(t, azure.VirtualMachineExists(t, vmName, resourceGroupName, subscriptionID))
}

func TestNICExistsAndConnected(t *testing.T) {
	// Setup Terraform resources
	setupTerraform(t)

	// Confirm NIC exists
	assert.True(t, azure.NetworkInterfaceExists(t, nicName, resourceGroupName, subscriptionID), "NIC does not exist")

	// Confirm NIC is attached to VM
	vm, err := azure.GetVirtualMachineE(vmName, resourceGroupName, subscriptionID)
	require.NoError(t, err, "Failed to get VM details")
	if vm.NetworkProfile.NetworkInterfaces == nil {
		t.Fatal("NetworkInterfaces is nil")
	}
	nicIDs := []string{}
	for _, nicRef := range *vm.NetworkProfile.NetworkInterfaces {
		if nicRef.ID != nil {
			nicIDs = append(nicIDs, *nicRef.ID)
		}
	}
	expectedNICID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkInterfaces/%s", subscriptionID, resourceGroupName, nicName)
	assert.Contains(t, nicIDs, expectedNICID, "NIC is not attached to VM")
}

func TestUbuntuVersion(t *testing.T) {
	// Setup Terraform resources
	setupTerraform(t)

	// Retrieve VM details
	vm, err := azure.GetVirtualMachineE(vmName, resourceGroupName, subscriptionID)
	require.NoError(t, err, "Failed to get VM details")

	// Confirm Ubuntu version
	imageRef := vm.StorageProfile.ImageReference
	assert.Equal(t, "Canonical", *imageRef.Publisher, "VM publisher is not Canonical")
	assert.Equal(t, "0001-com-ubuntu-server-jammy", *imageRef.Offer, "VM offer is not 0001-com-ubuntu-server-jammy")
	expectedUbuntuSku := "22_04-lts-gen2" // Matches main.tf
	assert.Equal(t, expectedUbuntuSku, *imageRef.Sku, "VM is not running Ubuntu 22.04 LTS Gen2")
}
