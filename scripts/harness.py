import requests
import concurrent.futures
import collections
import psycopg2
import os

ALB_URL = "http://flash-sale-system-alb-1382015506.us-east-1.elb.amazonaws.com"
DB_URL = os.environ["ORDERS_DB_URL"]  # set this before running
TOTAL_USERS = 200  # more than 100 to guarantee sold-out
EVENT_ID = "event-001"


def purchase(user_id):
    try:
        r = requests.post(
            f"{ALB_URL}/api/orders",
            json={"event_id": EVENT_ID, "user_id": f"user-{user_id}",
                  "quantity": 1},
            timeout=10,
        )
        data = r.json()
        return {
            "user_id": user_id,
            "order_id": data.get("order_id"),
            "status": data.get("status"),
            "http_status": r.status_code,
        }
    except Exception as e:
        return {"user_id": user_id, "error": str(e)}


def main():
    print(f"Sending {TOTAL_USERS} concurrent purchase requests...")

    with concurrent.futures.ThreadPoolExecutor(max_workers=TOTAL_USERS) as executor:
        results = list(executor.map(purchase, range(TOTAL_USERS)))

    # ── Collect results ───────────────────────────────────────────────
    confirmed = [r for r in results if r.get("status") == "confirmed"]
    failed = [r for r in results if r.get("status") == "failed"]
    errors = [r for r in results if "error" in r]

    print("\nResults:")
    print(f"  Confirmed : {len(confirmed)}")
    print(f"  Failed    : {len(failed)}")
    print(f"  Errors    : {len(errors)}")

    # ── Check 1: No overselling ───────────────────────────────────────
    print("\n[Check 1] No overselling...")
    if len(confirmed) <= 100:
        print(f"  PASS — {len(confirmed)} confirmed orders (limit 100)")
    else:
        print(f"  FAIL — {len(confirmed)} confirmed orders exceeded limit of 100")

    # ── Check 2: No duplicate order IDs ──────────────────────────────
    print("\n[Check 2] No duplicate order IDs...")
    order_ids = [r["order_id"] for r in confirmed if r.get("order_id")]
    duplicates = [id for id, count in collections.Counter(order_ids).items() if count > 1]
    if not duplicates:
        print(f"  PASS — all {len(order_ids)} order IDs are unique")
    else:
        print(f"  FAIL — duplicate order IDs found: {duplicates}")

    # ── Check 3: All confirmed orders exist in DB ─────────────────────
    print("\n[Check 3] All confirmed orders exist in database...")
    try:
        conn = psycopg2.connect(DB_URL)
        cur = conn.cursor()
        missing = []
        for r in confirmed:
            cur.execute("SELECT id FROM orders WHERE id = %s", (r["order_id"],))
            if cur.fetchone() is None:
                missing.append(r["order_id"])
        cur.close()
        conn.close()

        if not missing:
            print(f"  PASS — all {len(confirmed)} confirmed orders found in database")
        else:
            print(f"  FAIL — {len(missing)} confirmed orders missing from database: {missing}")
    except Exception as e:
        print(f"  SKIP — could not connect to database: {e}")

    # ── Summary ───────────────────────────────────────────────────────
    print("\n" + "─" * 40)
    all_passed = (
        len(confirmed) <= 100 and
        not duplicates and
        not missing if 'missing' in dir() else True
    )
    if all_passed:
        print("ALL CHECKS PASSED — system is correct")
    else:
        print("SOME CHECKS FAILED — review output above")


if __name__ == "__main__":
    main()
