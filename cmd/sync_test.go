/*
 * JuiceFS, Copyright 2021 Juicedata, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

 package cmd

 import (
	 "bytes"
	 "fmt"
	 "os"
	 "path/filepath"
	 "syscall"
	 "testing"
	 "time"
 
	 "github.com/juicedata/juicefs/pkg/object"
 )
 
 func TestSync(t *testing.T) {
	 if os.Getenv("MINIO_TEST_BUCKET") == "" {
		 t.Skip()
	 }
	 minioDir := "synctest"
	 localDir := "/tmp/synctest"
	 defer os.RemoveAll(localDir)
	 storage, err := object.CreateStorage("minio", os.Getenv("MINIO_TEST_BUCKET"), os.Getenv("MINIO_ACCESS_KEY"), os.Getenv("MINIO_SECRET_KEY"), "")
	 if err != nil {
		 t.Fatalf("create storage failed: %v", err)
	 }
 
	 testInstances := []struct{ path, content string }{
		 {"t1.txt", "content1"},
		 {"testDir1/t2.txt", "content2"},
		 {"testDir1/testDir3/t3.txt", "content3"},
	 }
 
	 for _, instance := range testInstances {
		 err = storage.Put(fmt.Sprintf("/%s/%s", minioDir, instance.path), bytes.NewReader([]byte(instance.content)))
		 if err != nil {
			 t.Fatalf("storage put failed: %v", err)
		 }
	 }
	 syncArgs := []string{"", "sync", fmt.Sprintf("minio://%s/%s", os.Getenv("MINIO_TEST_BUCKET"), minioDir), fmt.Sprintf("file://%s", localDir)}
	 err = Main(syncArgs)
	 if err != nil {
		 t.Fatalf("sync failed: %v", err)
	 }
 
	 for _, instance := range testInstances {
		 c, err := os.ReadFile(fmt.Sprintf("%s/%s", localDir, instance.path))
		 if err != nil || string(c) != instance.content {
			 t.Fatalf("sync failed: %v", err)
		 }
	 }
 }
 
 func TestSyncInterruptCleanup(t *testing.T) {
	 // Setup test directories
	 srcDir := t.TempDir()
	 dstDir := t.TempDir()
	 srcFile := filepath.Join(srcDir, "bigfile")
	 dstFile := filepath.Join(dstDir, "sync")
 
	 // Create a large source file
	 f, err := os.Create(srcFile)
	 if err != nil {
		 t.Fatalf("Create source file: %v", err)
	 }
	 if err := f.Truncate(1 << 30); err != nil { // 1GB
		 t.Fatalf("Truncate file: %v", err)
	 }
	 f.Close()
 
	 // Run sync in a goroutine
	 syncArgs := []string{"", "sync", "--verbose", fmt.Sprintf("file://%s", srcFile), fmt.Sprintf("file://%s", dstFile)}
	 syncDone := make(chan error)
	 go func() {
		 syncDone <- Main(syncArgs)
	 }()
 
	 // Wait briefly to ensure sync starts
	 time.Sleep(500 * time.Millisecond)
 
	 // Send SIGINT
	 if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		 t.Fatalf("Send SIGINT: %v", err)
	 }
 
	 // Wait for sync to exit
	 select {
	 case err := <-syncDone:
		 if err == nil {
			 t.Error("Expected sync to fail on interrupt")
		 }
	 case <-time.After(5 * time.Second):
		 t.Fatal("Sync did not exit after interrupt")
	 }
 
	 // Verify no temp files remain
	 matches, err := filepath.Glob(filepath.Join(dstDir, ".sync.tmp*"))
	 if err != nil {
		 t.Fatalf("Glob temp files: %v", err)
	 }
	 if len(matches) > 0 {
		 t.Errorf("Found %d temporary files: %v", len(matches), matches)
	 }
 
	 // Verify destination file does not exist
	 if _, err := os.Stat(dstFile); err == nil {
		 t.Error("Destination file exists after interrupt")
	 } else if !os.IsNotExist(err) {
		 t.Errorf("Stat destination: %v", err)
	 }
 }
 
 func Test_isS3PathType(t *testing.T) {
	 tests := []struct {
		 endpoint string
		 want     bool
	 }{
		 {"localhost", true},
		 {"localhost:8080", true},
		 {"127.0.0.1", true},
		 {"127.0.0.1:8080", true},
		 {"s3.ap-southeast-1.amazonaws.com", true},
		 {"s3.ap-southeast-1.amazonaws.com:8080", true},
		 {"s3-ap-southeast-1.amazonaws.com", true},
		 {"s3-ap-southeast-1.amazonaws.com:8080", true},
		 {"s3-ap-southeast-1.amazonaws..com:8080", false},
		 {"ap-southeast-1.amazonaws.com", false},
		 {"s3-ap-southeast-1amazonaws.com:8080", false},
		 {"s3-ap-southeast-1", false},
		 {"s3-ap-southeast-1:8080", false},
	 }
	 for _, tt := range tests {
		 t.Run("Test host", func(t *testing.T) {
			 if got := isS3PathType(tt.endpoint); got != tt.want {
				 t.Errorf("isS3PathType() = %v, want %v", got, tt.want)
			 }
		 })
	 }
 }
 
 func Test_extractToken(t *testing.T) {
	 // [NAME://][ACCESS_KEY:SECRET_KEY[:TOKEN]@]BUCKET[.ENDPOINT][/PREFIX]
	 tests := []struct {
		 uri, removedTokenUri, token string
	 }{
		 {"NAME://ACCESS_KEY:SECRET_KEY@BUCKET.ENDPOINT/PREFIX", "NAME://ACCESS_KEY:SECRET_KEY@BUCKET.ENDPOINT/PREFIX", ""},
		 {"NAME://:@BUCKET.ENDPOINT/PREFIX", "NAME://:@BUCKET.ENDPOINT/PREFIX", ""},
		 {"NAME://ACCESS_KEY:SECRET_KEY:TOKEN@BUCKET.ENDPOINT/PREFIX", "NAME://ACCESS_KEY:SECRET_KEY@BUCKET.ENDPOINT/PREFIX", "TOKEN"},
		 {"NAME://:@BUCKET.ENDPOINT/PREFIX", "NAME://:@BUCKET.ENDPOINT/PREFIX", ""},
		 {"NAME://::TOKEN@BUCKET.ENDPOINT/PREFIX", "NAME://:@BUCKET.ENDPOINT/PREFIX", "TOKEN"},
		 {"NAME://BUCKET.ENDPOINT/PREFIX", "NAME://BUCKET.ENDPOINT/PREFIX", ""},
		 {"file:///tmp/testbucket", "file:///tmp/testbucket", ""},
		 {"/tmp/testbucket", "/tmp/testbucket", ""},
	 }
	 for _, tt := range tests {
		 t.Run("", func(t *testing.T) {
			 removedTokenUri, token := extractToken(tt.uri)
			 if removedTokenUri != tt.removedTokenUri {
				 t.Errorf("extractToken() removedTokenUri = %v, want %v", removedTokenUri, tt.removedTokenUri)
			 }
			 if token != tt.token {
				 t.Errorf("extractToken() token = %v, want %v", token, tt.token)
			 }
		 })
	 }
 }