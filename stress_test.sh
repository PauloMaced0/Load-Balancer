#!/usr/bin/env bash

N=100   # total requests
C=5     # number of concurrent clients
MIN=100 # start precision

while getopts "n:c:m:" opt; do
  case ${opt} in
    n) N=$OPTARG ;;   # total requests
    c) C=$OPTARG ;;   # number of clients
    m) MIN=$OPTARG ;; # min precision
    *)
       echo "Usage: $0 [-n total_requests] [-c concurrent_clients] [-m min_precision]"
       exit 1
       ;;
  esac
done

K=$((N / C))

echo -e "Stress test Load Balancer "

PID_LIST=""

START=$(date +%s)
for _ in $(seq "$C")
do
  MAX=$((MIN + K))
  curl -s "http://localhost:8080/[$MIN-$MAX]" > /dev/null 2>&1 & 
  PID=$!
  PID_LIST="$PID_LIST $PID"
  echo -e "Requests [$MIN-$MAX] ($PID)"
  MIN=$MAX
done

FAIL=0
for PID in $PID_LIST; 
do
  echo -e "Wait for $PID..."    
  wait "$PID" || (( "FAIL+=1" ))
done
END=$(date +%s)

if [ "$FAIL" == "0" ]; then 
  echo "YAY!"
  TIME=$(echo "($END - $START)" | bc -l)
  RPS=$(echo "$N / $TIME" | bc)
  echo -e "Requests Per Second = $RPS ($TIME)"
else 
  echo "FAIL! ($FAIL)" 
fi

exit $FAIL
