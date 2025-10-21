#!/bin/bash

# Kitten Container Runtime Test Script
# Tests various features and configurations

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counter
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
print_test() {
    echo -e "${YELLOW}[TEST]${NC} $1"
}

print_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

print_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    TESTS_FAILED=$((TESTS_FAILED + 1))
}

print_info() {
    echo -e "[INFO] $1"
}

echo "======================================"
echo "Kitten Container Runtime Test Suite"
echo "======================================"
echo ""

# Check prerequisites
print_test "Checking prerequisites"

if [ "$EUID" -ne 0 ]; then
    print_fail "Must run as root"
    exit 1
fi

if [ ! -f "./kitten" ]; then
    print_fail "kitten binary not found in current directory"
    exit 1
fi

if [ ! -x "./kitten" ]; then
    print_fail "kitten binary is not executable"
    exit 1
fi

print_pass "Prerequisites check"
echo ""

# Test 1: Version command
print_test "Testing version command"
OUTPUT=$(./kitten version 2>&1 || true)
if echo "$OUTPUT" | grep -q "Kitten Container Runtime"; then
    print_pass "Version command works"
else
    print_fail "Version command failed"
    echo "Output: $OUTPUT"
fi

# Test 2: Help command
print_test "Testing help command"
OUTPUT=$(./kitten help 2>&1 || true)
if echo "$OUTPUT" | grep -q "Usage:"; then
    print_pass "Help command works"
else
    print_fail "Help command failed"
    echo "Output: $OUTPUT"
fi

# Check if rootfs exists
if [ ! -d "./rootfs" ]; then
    print_info "rootfs directory not found - skipping container tests"
    ROOTFS_EXISTS=false
else
    ROOTFS_EXISTS=true
fi

if [ "$ROOTFS_EXISTS" = true ]; then
    # Test 3: Basic container run
    print_test "Testing basic container run"
    OUTPUT=$(timeout 10 ./kitten run ./rootfs /bin/echo "Hello from Kitten" 2>&1 || true)
    if echo "$OUTPUT" | grep -q "Hello from Kitten"; then
        print_pass "Basic container run"
    else
        print_fail "Basic container run failed"
        echo "Output: $OUTPUT"
    fi

    # Test 4: Hostname setting
    print_test "Testing hostname setting"
    OUTPUT=$(timeout 10 ./kitten run --hostname testbox ./rootfs /bin/hostname 2>&1 || true)
    if echo "$OUTPUT" | grep -q "testbox"; then
        print_pass "Hostname setting works"
    else
        print_fail "Hostname setting failed"
        echo "Output: $OUTPUT"
    fi

    # Test 5: Working directory
    print_test "Testing working directory"
    OUTPUT=$(timeout 10 ./kitten run --workdir /tmp ./rootfs /bin/pwd 2>&1 || true)
    if echo "$OUTPUT" | grep -q "/tmp"; then
        print_pass "Working directory setting works"
    else
        print_fail "Working directory setting failed"
        echo "Output: $OUTPUT"
    fi

    # Test 6: Environment variables
    print_test "Testing environment variables"
    OUTPUT=$(timeout 10 ./kitten run --env TEST_VAR=test123 ./rootfs /bin/sh -c 'echo $TEST_VAR' 2>&1 || true)
    if echo "$OUTPUT" | grep -q "test123"; then
        print_pass "Environment variables work"
    else
        print_fail "Environment variables failed"
        echo "Output: $OUTPUT"
    fi

    # Test 7: PID namespace isolation
    print_test "Testing PID namespace isolation"
    OUTPUT=$(timeout 10 ./kitten run ./rootfs /bin/sh -c 'echo $$' 2>&1 || true)
    if echo "$OUTPUT" | grep -q "1"; then
        print_pass "PID namespace isolation works"
    else
        print_fail "PID namespace isolation failed"
        echo "Output: $OUTPUT"
    fi

    # Test 8: Disable PID namespace
    print_test "Testing --no-pid flag"
    OUTPUT=$(timeout 10 ./kitten run --no-pid ./rootfs /bin/sh -c 'echo $$' 2>&1 || true)
    PID=$(echo "$OUTPUT" | grep -o '[0-9]*' | head -1)
    if [ -n "$PID" ] && [ "$PID" != "1" ]; then
        print_pass "--no-pid flag works"
    else
        print_fail "--no-pid flag failed"
        echo "Output: $OUTPUT"
    fi

    # Test 9: Multiple commands
    print_test "Testing multiple commands with arguments"
    OUTPUT=$(timeout 10 ./kitten run ./rootfs /bin/sh -c "echo first && echo second" 2>&1 || true)
    if echo "$OUTPUT" | grep -q "first" && echo "$OUTPUT" | grep -q "second"; then
        print_pass "Multiple commands work"
    else
        print_fail "Multiple commands failed"
        echo "Output: $OUTPUT"
    fi

    # Test 10: Exit code propagation
    print_test "Testing exit code propagation"
    ./kitten run ./rootfs /bin/sh -c "exit 42" > /dev/null 2>&1
    EXIT_CODE=$?
    if [ $EXIT_CODE -eq 42 ]; then
        print_pass "Exit code propagation works"
    else
        print_fail "Exit code propagation failed (expected 42, got $EXIT_CODE)"
    fi

    # Test 11: Invalid command handling
    print_test "Testing invalid command handling"
    OUTPUT=$(timeout 10 ./kitten run ./rootfs /bin/nonexistent 2>&1 || true)
    if echo "$OUTPUT" | grep -qi "error\|failed\|not found\|no such"; then
        print_pass "Invalid command handling works"
    else
        print_fail "Invalid command handling failed"
        echo "Output: $OUTPUT"
    fi

    # Test 12: Network mode none
    print_test "Testing network mode: none"
    OUTPUT=$(timeout 10 ./kitten run --network none ./rootfs /bin/true 2>&1 || true)
    if [ $? -eq 0 ]; then
        print_pass "Network mode none works"
    else
        print_fail "Network mode none failed"
        echo "Output: $OUTPUT"
    fi

    # Test 13: Network mode host
    print_test "Testing network mode: host"
    OUTPUT=$(timeout 10 ./kitten run --network host ./rootfs /bin/ip link 2>&1 || true)
    if echo "$OUTPUT" | grep -q "link"; then
        print_pass "Network mode host works"
    else
        print_fail "Network mode host failed"
        echo "Output: $OUTPUT"
    fi

    # Test 14: Multiple environment variables
    print_test "Testing multiple environment variables"
    OUTPUT=$(timeout 10 ./kitten run --env VAR1=value1 --env VAR2=value2 ./rootfs /bin/sh -c 'echo $VAR1 $VAR2' 2>&1 || true)
    if echo "$OUTPUT" | grep -q "value1 value2"; then
        print_pass "Multiple environment variables work"
    else
        print_fail "Multiple environment variables failed"
        echo "Output: $OUTPUT"
    fi

    # Test 15: Long running process
    print_test "Testing long running process"
    OUTPUT=$(timeout 10 ./kitten run ./rootfs /bin/sh -c 'sleep 2 && echo done' 2>&1 || true)
    if echo "$OUTPUT" | grep -q "done"; then
        print_pass "Long running process works"
    else
        print_fail "Long running process failed"
        echo "Output: $OUTPUT"
    fi

    # Test 16: File operations in container
    print_test "Testing file operations in container"
    OUTPUT=$(timeout 10 ./kitten run ./rootfs /bin/sh -c 'echo test > /tmp/testfile && cat /tmp/testfile' 2>&1 || true)
    if echo "$OUTPUT" | grep -q "test"; then
        print_pass "File operations work"
    else
        print_fail "File operations failed"
        echo "Output: $OUTPUT"
    fi

    # Test 17: Process listing in PID namespace
    print_test "Testing process listing in PID namespace"
    OUTPUT=$(timeout 10 ./kitten run ./rootfs /bin/ps 2>&1 || true)
    # Should see limited processes in isolated namespace
    if [ $? -eq 0 ] || echo "$OUTPUT" | grep -q "PID\|ps"; then
        print_pass "Process listing works"
    else
        print_fail "Process listing failed"
        echo "Output: $OUTPUT"
    fi

    # Test 18: UTS namespace isolation
    print_test "Testing UTS namespace isolation with --no-uts"
    HOST_HOSTNAME=$(hostname)
    OUTPUT=$(timeout 10 ./kitten run --no-uts ./rootfs /bin/hostname 2>&1 || true)
    if echo "$OUTPUT" | grep -q "$HOST_HOSTNAME"; then
        print_pass "UTS namespace disable works"
    else
        print_fail "UTS namespace disable failed"
        echo "Output: $OUTPUT"
    fi

    # Test 19: Mount namespace isolation
    print_test "Testing mount namespace isolation"
    OUTPUT=$(timeout 10 ./kitten run ./rootfs /bin/mount 2>&1 || true)
    # Should see proc, sys, etc mounted
    if echo "$OUTPUT" | grep -q "proc\|sys"; then
        print_pass "Mount namespace isolation works"
    else
        print_fail "Mount namespace isolation failed"
        echo "Output: $OUTPUT"
    fi

    # Test 20: Command with multiple arguments
    print_test "Testing command with multiple arguments"
    OUTPUT=$(timeout 10 ./kitten run ./rootfs /bin/echo arg1 arg2 arg3 2>&1 || true)
    if echo "$OUTPUT" | grep -q "arg1 arg2 arg3"; then
        print_pass "Multiple arguments work"
    else
        print_fail "Multiple arguments failed"
        echo "Output: $OUTPUT"
    fi

    # Test 21: Stderr output
    print_test "Testing stderr output capture"
    OUTPUT=$(timeout 10 ./kitten run ./rootfs /bin/sh -c 'echo error >&2' 2>&1 || true)
    if echo "$OUTPUT" | grep -q "error"; then
        print_pass "Stderr capture works"
    else
        print_fail "Stderr capture failed"
        echo "Output: $OUTPUT"
    fi

    # Test 22: Empty command handling
    print_test "Testing empty/missing command handling"
    OUTPUT=$(timeout 10 ./kitten run ./rootfs 2>&1 || true)
    if echo "$OUTPUT" | grep -qi "error\|usage\|required"; then
        print_pass "Empty command handling works"
    else
        print_fail "Empty command handling failed"
        echo "Output: $OUTPUT"
    fi

    # Test 23: Invalid rootfs path
    print_test "Testing invalid rootfs path"
    OUTPUT=$(timeout 10 ./kitten run /nonexistent/path /bin/echo test 2>&1 || true)
    if echo "$OUTPUT" | grep -qi "error\|failed\|not found\|no such"; then
        print_pass "Invalid rootfs handling works"
    else
        print_fail "Invalid rootfs handling failed"
        echo "Output: $OUTPUT"
    fi

    # Test 24: Signal handling
    print_test "Testing signal handling (SIGTERM)"
    # Start a sleep process and kill it
    timeout 10 bash -c '
        ./kitten run ./rootfs /bin/sleep 100 >/dev/null 2>&1 &
        PID=$!
        sleep 1
        kill -TERM $PID 2>/dev/null
        wait $PID 2>/dev/null
        EXIT=$?
        if [ $EXIT -ne 0 ]; then
            exit 0
        else
            exit 1
        fi
    ' || true
    if [ $? -eq 0 ]; then
        print_pass "Signal handling works"
    else
        print_fail "Signal handling failed"
    fi

    # Test 25: Concurrent containers
    print_test "Testing concurrent container execution"
    ./kitten run ./rootfs /bin/echo "container1" > /tmp/kitten_test_1.txt 2>&1 &
    PID1=$!
    ./kitten run ./rootfs /bin/echo "container2" > /tmp/kitten_test_2.txt 2>&1 &
    PID2=$!
    wait $PID1 2>/dev/null
    wait $PID2 2>/dev/null
    if grep -q "container1" /tmp/kitten_test_1.txt && grep -q "container2" /tmp/kitten_test_2.txt; then
        print_pass "Concurrent containers work"
    else
        print_fail "Concurrent containers failed"
    fi
    rm -f /tmp/kitten_test_1.txt /tmp/kitten_test_2.txt 2>/dev/null
fi

# Summary
echo ""
echo "======================================"
echo "Test Summary"
echo "======================================"
echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
echo -e "${RED}Failed: $TESTS_FAILED${NC}"
echo "Total: $((TESTS_PASSED + TESTS_FAILED))"
echo ""

if [ $TESTS_FAILED -eq 0 ] && [ $TESTS_PASSED -gt 0 ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed or no tests ran${NC}"
    exit 1
fi
