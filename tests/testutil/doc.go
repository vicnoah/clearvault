// Package testutil provides utilities for ClearVault integration testing.
//
// This package includes tools for managing test servers (zs3 for S3, sweb for WebDAV)
// and provides helper functions to simplify integration test writing.
//
// Quick Start:
//
// Use the testutil.EnsureServersForTest function to automatically start test servers
// for your integration tests:
//
//	func TestMyFeature(t *testing.T) {
//	    manager := testutil.EnsureServersForTest(t)
//	    // Servers are now running and will be cleaned up after the test
//	    // Use manager.GetZS3Endpoint(), manager.GetWebDAVURL() to get connection info
//	}
//
// For package-level test setup (recommended for multiple tests), use TestMain:
//
//	func TestMain(m *testing.M) {
//	    manager, err := testutil.NewTestServerManager()
//	    if err != nil {
//	        panic(err)
//	    }
//	    if err := manager.StartAll(); err != nil {
//	        panic(err)
//	    }
//	    code := m.Run()
//	    manager.StopAll()
//	    os.Exit(code)
//	}
//
// Server Configuration:
//
// Default configuration:
//   - ZS3 (S3): localhost:9000, access-key: minioadmin, secret-key: minioadmin
//   - SWEB (WebDAV): localhost:8081/webdav, user: admin, pass: admin123
//
// Test server binaries should be located in the project's tools/ directory.
// You can install them using:
//   ./scripts/install_zs3.sh
//   ./scripts/install_sweb.sh
//
package testutil
