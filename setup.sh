#!/usr/bin/env bash

N_SERVERS=${1:-4} 
POLICY=${2:-N2One}

# delete old pid file
rm -f http_server.pid

mkdir -p build
go build -o build/server ./src/cmd/server
go build -o build/load_balancer ./src/cmd/load_balancer

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
