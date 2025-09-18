#!/usr/bin/env bash

N_SERVERS=4
POLICY="N2One"

while getopts "n:p:" opt; do
  case ${opt} in
    n) N_SERVERS=$OPTARG ;;  # number of servers
    p) POLICY=$OPTARG ;;     # scheduling policy
    *) 
      echo "Usage: $0 [-n number_of_servers] [-p policy]"
      exit 1
      ;;
  esac
done

# delete old pid file
rm -f http_server.pid

cd src ... || exit

mkdir -p build
go build -o build/server ./cmd/server
go build -o build/load_balancer ./cmd/load_balancer

echo -e Start "$N_SERVERS" Http Servers

SERVERS=""

for ((i = 0 ; i < "$N_SERVERS" ; i++ ));
do 
  PORT=$((8000 + i))
  SERVERS="$SERVERS""localhost:$PORT "
  ./build/server -p $PORT > /dev/null 2>&1 & 
  PID=$! 
  echo $PID >> http_server.pid
  echo -e "\tStart HTTP Server localhost:$PORT ($PID)"
done

echo -e Start Load Balancer: "$SERVERS"

./build/load_balancer -p 8080 -s "$SERVERS" -a "$POLICY"

echo -e Stop "$N_SERVERS" Http Servers

while read -r PID; do
  echo -e "\tKill -9 $PID"
  kill -9 "$PID"
  wait "$PID" 2>/dev/null
done < http_server.pid

rm -f http_server.pid
