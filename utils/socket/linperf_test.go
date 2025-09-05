package socket

import "testing"

func Test_getLinperfDataFileName(t *testing.T) {
	outputFile := getLinperfDataFileName(`2025-07-23 13:34:54 Compressing the following files into linperf_RESULTS_sample.20250723.133044.tar.gz.
2025-07-23 13:34:54 \tTemporary directory linperf_RESULTS_sample.20250723.133044 removed.
2025-07-23 13:34:54 \tlinperf script complete.
To share with IBM support, upload all the following files:
* /serviceability/olo-test/example-75dfd65979-mwvnz/performanceData/linperf_RESULTS_sample.20250723.133044
* /var/log/messages (Linux OS files)
For WebSphere Application Server:
* Logs (systemout.log, native_stderr.log, etc)
* server.xml for the server(s) that you are providing data for
For Liberty:
* Logs (messages.log, console.log, etc)
* server.env, server.xml, and jvm.options`)
	tests := []Test{
		{"capture output file", "/serviceability/olo-test/example-75dfd65979-mwvnz/performanceData/linperf_RESULTS_sample.20250723.133044.tar.gz", outputFile},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func Test_getSubstring(t *testing.T) {
	substr1 := `hello world`
	tests := []Test{
		{"check substring 1", " wo", getSubstring([]string{substr1}, "ello", "wo")},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}
