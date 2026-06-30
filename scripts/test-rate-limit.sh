#!/bin/bash

# Port or URL of the API Gateway
GATEWAY_URL="http://localhost:11111"
ENDPOINT="/v1/auth/login"
REQUESTS=110
CONCURRENT=5

echo "============================================="
echo "   API Gateway Rate Limiting Test Script"
echo "============================================="
echo "Target Endpoint: $GATEWAY_URL$ENDPOINT"
echo "Total Requests:  $REQUESTS"
echo "Concurrency:     $CONCURRENT"
echo "============================================="

# Create temporary file to collect status codes
STATUS_FILE=$(mktemp)

# Cleanup on exit
trap 'rm -f "$STATUS_FILE"' EXIT

# Function to make requests
make_request() {
  local id=$1
  # Make request and extract HTTP status code
  status=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL$ENDPOINT")
  echo "$status" >> "$STATUS_FILE"
}

export -f make_request
export GATEWAY_URL
export ENDPOINT
export STATUS_FILE

# Run requests using xargs in parallel to speed up testing
seq 1 "$REQUESTS" | xargs -n 1 -P "$CONCURRENT" -I {} bash -c 'make_request "$@"' _ {}

# Parse results
total_sent=$(wc -l < "$STATUS_FILE")
success_200=$(grep -c "200" "$STATUS_FILE" 2>/dev/null || echo 0)
bad_req_400=$(grep -c "400" "$STATUS_FILE" 2>/dev/null || echo 0) # login with no body might return 400
rate_limit_429=$(grep -c "429" "$STATUS_FILE" 2>/dev/null || echo 0)
others=$(grep -vE "200|400|429" "$STATUS_FILE" | wc -l)

echo ""
echo "----------------- RESULTS -----------------"
echo "Total Requests Sent:   $total_sent"
echo "Status 200 (Success):  $success_200"
echo "Status 400 (Bad Req):  $bad_req_400"
echo "Status 429 (Blocked):  $rate_limit_429"
if [ "$others" -gt 0 ]; then
  echo "Other Status Codes:    $others"
fi
echo "-------------------------------------------"

if [ "$rate_limit_429" -gt 0 ]; then
  echo "✅ Rate Limiting works! Blocked $rate_limit_429 requests with 429 (Too Many Requests)."
else
  echo "❌ Rate Limiting DID NOT trigger (0 requests blocked with 429)."
  echo "Note: The rate limit threshold is 100 requests per minute. You may need to run this script again or increase the REQUESTS count."
fi
echo "============================================="
