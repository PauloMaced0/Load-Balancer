# Go HTTP Server + Load Balancer

This project implements a **simple HTTP server** that computes an approximation of π using the **Leibniz formula**, together with a **TCP load balancer** that supports multiple scheduling policies.

The system consists of:
- An HTTP server (`server`) that serves π approximations based on a requested precision.
- A load balancer (`load_balancer`) that distributes requests across multiple HTTP servers using different scheduling policies.
- Helper bash scripts for starting multiple servers with the load balancer, and for stress-testing the system.

---

## Components

### 1. HTTP Server

- Listens on a configurable port (`-p` flag, default: `8080`).
- Computes π using the **Leibniz series** with a given precision.
- Enforces **single-threaded request processing** using a `sync.Mutex`.

### 2. Load Balancer 

- Listens for incoming TCP connections and proxies traffic to backend servers.
- Supports the following policies:
    - **N2One**: always forwards to the first server.
    - **RoundRobin**: cycles through all servers.
    - **LeastConnections**: selects the server with the fewest active connections.
    - **LeastResponseTime**: chooses based on average response time.
    
### 3. Setup Script (`setup.sh`)

Automates:

- Building the binaries.
- Starting multiple HTTP servers.
- Launching the load balancer.

**Example**:

```bash
./run.sh -n 4 -p RoundRobin
```

Starts 4 HTTP servers on ports `8000-8003`, and runs a load balancer on port `8080` using `Round Robin` scheduling.

### 4. Stress Test Script (`stress_test.sh`)

Simulates concurrent requests to the load balancer.

**Example**:

```bash
./stress.sh -n 100 -c 5 -m 100
```

- Sends 100 total requests.

- Uses 5 concurrent clients.

- Each client starts with a precision of 100.

Outputs requests per second and reports if any requests failed.

---

## How to Run

### 1. Clone & Build

**1**. Clone the repo

```bash
git clone git@github.com:PauloMaced0/Load-Balancer.git
cd Load-Balancer/src 
```

**2**. Fetch and tidy dependencies

```bash
go mod tidy
```

**3**. Build binaries

```bash
mkdir -p build 
go build -o build/server ./cmd/server
go build -o build/load_balancer ./cmd/load_balancer
```

### 2. Start Servers & Load Balancer

```bash
./setup.sh -n 4 -p RoundRobin 
```

- Starts 5 servers (`localhost:8000,8001,8002,8003,8004`).

- Runs load balancer on port `8080` using `Round Robin`.

### 3. Send Requests

```bash
curl http://localhost:8080/500
```

### 4. Run Stress Test

```bash
./scripts/stress.sh -n 200 -c 10 -m 100
```

> [!NOTE]
> The π server simulates computation delays (1ms per iteration) to make load balancing observable.
