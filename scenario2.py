import subprocess
import time
import os

os.environ["SCENARIO"] = "scenario2"
os.makedirs("logs/scenario2", exist_ok=True)

def run(cmd):
    print(f"  > {cmd}")
    result = subprocess.run(cmd, shell=True, capture_output=True, text=True, env=os.environ)
    print(result.stdout.strip())
    return result.stdout

def client(server, *args):
    return run(f"docker-compose run --rm client -server {server} {' '.join(args)}")

def find_leader(candidates, max_wait=90):
    print("  Finding current leader...")
    deadline = time.time() + max_wait
    attempt = 0
    while time.time() < deadline:
        attempt += 1
        for s in candidates:
            out = client(s, "deposit ACC00000001 1")
            if "ok:" in out:
                print(f"  Leader is {s}")
                return s
        remaining = int(deadline - time.time())
        print(f"  No leader found (attempt {attempt}, {remaining}s left) — retrying in 3s...")
        time.sleep(3)
    return None

ALL = ["server1:8080", "server2:8080", "server3:8080", "server4:8080", "server5:8080"]

print("=== Scenario 2: Raft Leader Election Demo ===")
print("    Logs will be written to logs/scenario2/")

# 1. Start 5 Servers: S1, S2, S3, S4 and S5.
print("\n[1] Start 5 Servers: S1, S2, S3, S4 and S5.")
run("docker-compose up --build -d")
print("    Waiting for all servers to be ready...")
time.sleep(6)
client("server1:8080", "open-account teller1 Alice 1000")
client("server1:8080", "open-account teller1 Bob 500")

# 2. Elect any random leader (e.g. S1). Stop any two follower servers
#    (e.g. S4, S5) during or after elections, but before any replication.
print("\n[2] Elect any random leader (e.g. S1). Stop any two follower servers (e.g. S4, S5) during or after elections, but before any replication.")
print("    S1 is already leader (started with -leader flag).")
run("docker-compose stop server4")
run("docker-compose stop server5")

# 3. Wait for at least 20 clients' requests (any type).
# 4. Stop the leader before replicating all entries (make sure to have
#    only 10 of them committed).
# Note: send 10 requests (commit to S1/S2/S3), stop S1, then send 10 more
# that fail — demonstrating that only 10 of the 20 are committed.
print("\n[3] Wait for at least 20 clients' requests (any type).")
for i in range(1, 11):
    print(f"  Request {i} of 20")
    client("server1:8080", "deposit ACC00000001 100")

print("\n[4] Stop the leader before replicating all entries (make sure to have only 10 of them committed).")
run("docker-compose stop server1")
print("    10 entries committed. Sending remaining 10 requests to show they fail without a leader...")
for i in range(11, 21):
    print(f"  Request {i} of 20 (expected to fail — no leader)")
    client("server2:8080", "deposit ACC00000001 100")

# 5. Start an election (should fail because only two servers up).
print("\n[5] Start an election (should fail because only two servers up).")
print("    Only S2 and S3 are up — no majority possible. Waiting to confirm election fails...")
time.sleep(10)

# 6. Start one of the broken followers (e.g. S4).
print("\n[6] Start one of the broken followers (e.g. S4).")
run("docker-compose start server4")
time.sleep(3)

# 7. Elect another leader (e.g. S2 for the second term).
print("\n[7] Elect another leader (e.g. S2 for the second term).")
print("    Waiting for election to complete...")
time.sleep(5)
leader2 = find_leader(["server2:8080", "server3:8080", "server4:8080"])
if leader2 is None:
    print("ERROR: No leader elected. Check server logs in logs/scenario2/.")
    raise SystemExit(1)

# 8. Wait for at least 30 clients' requests (any type).
# 9. Stop the leader before replicating all entries (make sure to have
#    only 20 of them committed).
# Note: send 20 requests (commit), stop the leader, then send 10 more that fail.
print("\n[8] Wait for at least 30 clients' requests (any type).")
for i in range(1, 21):
    print(f"  Request {i} of 30")
    client(leader2, "deposit ACC00000001 100")

print("\n[9] Stop the leader before replicating all entries (make sure to have only 20 of them committed).")
stop_name = leader2.replace(":8080", "")
run(f"docker-compose stop {stop_name}")
print("    20 entries committed. Sending remaining 10 requests to show they fail without a leader...")
remaining = [s for s in ["server2:8080", "server3:8080", "server4:8080"] if s != leader2][0]
for i in range(21, 31):
    print(f"  Request {i} of 30 (expected to fail — no leader)")
    client(remaining, "deposit ACC00000001 100")

# 10. Start the previous leader (S1) again.
print("\n[10] Start the previous leader (S1) again.")
run("docker-compose start server1")
time.sleep(3)

# 11. Do another election and keep the system running, accept 40 new requests.
print("\n[11] Do another election and keep the system running, accept 40 new requests.")
print("    Waiting for election to complete...")
time.sleep(5)
leader3 = find_leader(["server1:8080", "server2:8080", "server3:8080", "server4:8080"])
if leader3 is None:
    print("ERROR: No leader elected. Check server logs in logs/scenario2/.")
    raise SystemExit(1)
for i in range(1, 41):
    print(f"  Request {i} of 40")
    client(leader3, "deposit ACC00000001 100")

# 12. Start server S5 after all the requests are replicated on the other 4 servers.
print("\n[12] Start server S5 after all the requests are replicated on the other 4 servers.")
run("docker-compose start server5")
print("    Waiting for S5 to catch up via heartbeats...")
time.sleep(8)

# After the final round of requests, verify that all the logs are the same
# using the testing server.
print("\n[Final] After the final round of requests, verify that all the logs are the same using the testing server.")
run("docker-compose run --rm client compare-log -tester tester:9000")

run("docker-compose down")
print("\n=== Scenario 2 Complete — logs saved to logs/scenario2/ ===")
