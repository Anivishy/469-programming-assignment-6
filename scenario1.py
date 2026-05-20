import subprocess
import time
import os
import shutil

os.environ["SCENARIO"] = "scenario1"
shutil.rmtree("logs/scenario1", ignore_errors=True)
os.makedirs("logs/scenario1", exist_ok=True)

CLIENT_CONTAINER = "raft_demo_client"

def run(cmd):
    print(f"  > {cmd}")
    result = subprocess.run(cmd, shell=True, capture_output=True, text=True, env=os.environ)
    print(result.stdout.strip())
    return result.stdout

def client(server, *args):
    arg_str = ' '.join(args)
    print(f"  > /bank-client -server {server} {arg_str}")
    result = subprocess.run(
        f"docker exec {CLIENT_CONTAINER} /bank-client -server {server} {arg_str}",
        shell=True, capture_output=True, text=True, env=os.environ
    )
    out = result.stdout.strip()
    print(out)
    return result.stdout

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

print("=== Scenario 1: Raft Leader Election Demo ===")
print("    Logs will be written to logs/scenario1/")

# 1. Start 5 Servers: S1, S2, S3, S4 and S5.
print("\n[1] Start 5 Servers: S1, S2, S3, S4 and S5.")
run("docker-compose up --build -d")
print("    Waiting for all servers to be ready...")
time.sleep(6)
run(f"docker rm -f {CLIENT_CONTAINER} 2>nul || true")
run(f"docker-compose run -d --name {CLIENT_CONTAINER} --entrypoint sleep client infinity")
client("server1:8080", "open-account teller1 Alice 1000")
client("server1:8080", "open-account teller1 Bob 500")

# 2. Elect any random leader (e.g. S1). Stop any follower server (e.g. S5)
#    during or after elections, but before any replication.
print("\n[2] Elect any random leader (e.g. S1). Stop any follower server (e.g. S5) during or after elections, but before any replication.")
print("    S1 is already leader (started with -leader flag).")
run("docker-compose stop server5")

# 3. Wait for at least 20 clients' requests (any type).
print("\n[3] Wait for at least 20 clients' requests (any type).")
for i in range(1, 21):
    print(f"  Request {i} of 20")
    client("server1:8080", "deposit ACC00000001 100")

# 4. Stop the leader before replicating all entries.
print("\n[4] Stop the leader before replicating all entries.")
run("docker-compose stop server1")

# 5. Elect another leader (e.g. S2 for the second term).
print("\n[5] Elect another leader (e.g. S2 for the second term).")
print("    Waiting for election to complete...")
time.sleep(5)
leader2 = find_leader(["server2:8080", "server3:8080", "server4:8080"])
if leader2 is None:
    print("ERROR: No leader elected. Check server logs in logs/scenario1/.")
    raise SystemExit(1)

# 6. Wait for at least 30 clients' requests (any type).
print("\n[6] Wait for at least 30 clients' requests (any type).")
for i in range(1, 31):
    print(f"  Request {i} of 30")
    client(leader2, "deposit ACC00000001 100")

# 7. Stop the leader before replicating all entries.
print("\n[7] Stop the leader before replicating all entries.")
stop_name = leader2.replace(":8080", "")
run(f"docker-compose stop {stop_name}")

# 8. Start S5 and S1 again.
print("\n[8] Start S5 and S1 again.")
run("docker-compose start server5")
run("docker-compose start server1")

# 9. Do another election and keep the system running, accept 40 new requests.
print("\n[9] Do another election and keep the system running, accept 40 new requests.")
print("    Waiting for election to complete...")
time.sleep(5)
leader3 = find_leader(ALL)
if leader3 is None:
    print("ERROR: No leader elected. Check server logs in logs/scenario1/.")
    raise SystemExit(1)
for i in range(1, 41):
    print(f"  Request {i} of 40")
    client(leader3, "deposit ACC00000001 100")

# After the final round of requests, verify that all the logs are the same
# using the testing server.
print("\n[Final] After the final round of requests, verify that all the logs are the same using the testing server.")
run("docker-compose run --rm client compare-log -tester tester:9000")

run(f"docker stop {CLIENT_CONTAINER} && docker rm {CLIENT_CONTAINER}")
run("docker-compose down")
print("\n=== Scenario 1 Complete — logs saved to logs/scenario1/ ===")
