package socket

import "testing"

func TestCommaSeparatedStringContains(t *testing.T) {
	outputFile := getLinperfDataFileName(`2025-07-23 13:34:54 \tTemporary directory linperf_RESULTS.20250723.133044 removed.
2025-07-23 13:34:54 \tlinperf script complete.
To share with IBM support, upload all the following files:
* /serviceability/olo-test/example-75dfd65979-mwvnz/performanceData/linperf_RESULTS.20250723.133044.tar.gz
* /var/log/messages (Linux OS files)
For WebSphere Application Server:
* Logs (systemout.log, native_stderr.log, etc)
* server.xml for the server(s) that you are providing data for
For Liberty:
* Logs (messages.log, console.log, etc)
* server.env, server.xml, and jvm.options`)
	tests := []Test{
		{"capture output file", "/serviceability/olo-test/example-75dfd65979-mwvnz/performanceData/linperf_RESULTS.20250723.133044.tar.gz", outputFile},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}
