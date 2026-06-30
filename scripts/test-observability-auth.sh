#!/bin/bash
# scripts/test-observability-auth.sh
# Verifies that observability endpoints require admin credentials and allow access when token is provided.

GATEWAY_URL="http://localhost:11111"
echo "=== 1. Checking unauthenticated access (Expect: 401 Unauthorized) ==="
STATUS_LOGS=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/logs/stream")
STATUS_PROFILER=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/debug/profiler")

echo "Logs stream status: $STATUS_LOGS"
echo "Profiler status: $STATUS_PROFILER"

if [ "$STATUS_LOGS" -eq 401 ] && [ "$STATUS_PROFILER" -eq 401 ]; then
    echo "✅ Success: Unauthenticated access blocked with 401."
else
    echo "❌ Fail: Observability endpoints did not block unauthenticated access with 401."
    exit 1
fi

echo ""
echo "=== 2. Authenticating as admin ==="
LOGIN_RESP=$(curl -s -X POST "$GATEWAY_URL/v1/auth/login-admin" \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@admin.com","password":"123456Aa@"}')

TOKEN=$(echo "$LOGIN_RESP" | grep -oP '"token":"\K[^"]+')

if [ -n "$TOKEN" ]; then
    echo "✅ Success: Received admin JWT token."
else
    echo "❌ Fail: Failed to authenticate as admin. Response:"
    echo "$LOGIN_RESP"
    exit 1
fi

echo ""
echo "=== 3. Checking authenticated access using token ==="
# Test logs stream with token query param using --max-time 3 to avoid hanging
STATUS_LOGS_AUTH=$(curl -s -o /dev/null -w "%{http_code}" -m 3 "$GATEWAY_URL/logs/stream?token=$TOKEN&service=api-gateway")
# Test profiler stats with Authorization header
STATUS_PROFILER_AUTH=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $TOKEN" "$GATEWAY_URL/debug/profiler")

echo "Authenticated logs stream status (exit code/http code): $STATUS_LOGS_AUTH"
echo "Authenticated profiler status: $STATUS_PROFILER_AUTH"

# Since curl is interrupted by timeout, it might return 000 or 200 depending on when timeout fires.
# If it receives headers before timing out, it can print 200. Either 200 or 000 (interrupted) is fine as long as it's not 401/403.
if [ "$STATUS_LOGS_AUTH" -ne 401 ] && [ "$STATUS_LOGS_AUTH" -ne 403 ] && [ "$STATUS_PROFILER_AUTH" -eq 200 ]; then
    echo "✅ Success: Authenticated access granted."
else
    echo "❌ Fail: Authenticated access failed."
    exit 1
fi

echo ""
echo "=== 4. Testing profiler reset action with admin role ==="
STATUS_RESET=$(curl -s -o /dev/null -w "%{http_code}" -X POST -H "Authorization: Bearer $TOKEN" "$GATEWAY_URL/debug/profiler/reset?service=api-gateway")
echo "Profiler reset status: $STATUS_RESET"
if [ "$STATUS_RESET" -eq 200 ]; then
    echo "✅ Success: Profiling reset permitted."
else
    echo "❌ Fail: Reset stats returned $STATUS_RESET."
    exit 1
fi

echo ""
echo "🎉 All backend authentication tests passed successfully!"
exit 0
