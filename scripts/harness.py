import requests
import concurrent.futures
import collections
import threading
import psycopg2
import time

ALB_URL        = "http://flash-sale-system-alb-1588994481.us-east-1.elb.amazonaws.com"
ORDERS_DB_URL    = "postgresql://flashsalesystem:flashsalepassword@flash-sale-system-orders.cs78k4i80u4a.us-east-1.rds.amazonaws.com:5432/orders"
INVENTORY_DB_URL = "postgresql://flashsalesystem:flashsalepassword@flash-sale-system-inventory.cs78k4i80u4a.us-east-1.rds.amazonaws.com:5432/inventory"

TOTAL_USERS       = 200
INITIAL_INVENTORY = 100
EVENT_ID          = "event-001"

barrier = threading.Barrier(TOTAL_USERS)


def purchase(user_id):
    try:
        barrier.wait()  # all threads wait here until everyone is ready, then fire simultaneously
        r = requests.post(
            f"{ALB_URL}/api/orders",
            json={"event_id": EVENT_ID, "user_id": f"user-{user_id}", "quantity": 1},
            timeout=10,
        )
        data = r.json()
        return {
            "user_id":     user_id,
            "order_id":    data.get("order_id"),
            "status":      data.get("status"),
            "http_status": r.status_code,
        }
    except Exception as e:
        return {"user_id": user_id, "error": str(e)}


def wait_for_queue_drain():
    print("\nWaiting 5s for RabbitMQ notification queue to drain...")
    time.sleep(5)
    print("  Done waiting")


def main():
    print(f"Sending {TOTAL_USERS} concurrent purchase requests...")

    with concurrent.futures.ThreadPoolExecutor(max_workers=TOTAL_USERS) as executor:
        results = list(executor.map(purchase, range(TOTAL_USERS)))

    # ── Collect results ───────────────────────────────────────────────
    confirmed = [r for r in results if r.get("status") == "confirmed"]
    failed    = [r for r in results if r.get("status") == "failed"]
    errors    = [r for r in results if "error" in r]

    print(f"\nResults:")
    print(f"  Confirmed : {len(confirmed)}")
    print(f"  Failed    : {len(failed)}")
    print(f"  Errors    : {len(errors)}")
    if errors:
        print(f"\nSample error: {errors[0]}")

    # wait for notifications to finish processing before running checks
    wait_for_queue_drain()

    passes = []

    # ── Check 1: No overselling ───────────────────────────────────────
    print("\n[Check 1] No overselling (API responses)...")
    passed = len(confirmed) <= INITIAL_INVENTORY
    passes.append(passed)
    if passed:
        print(f"  PASS — {len(confirmed)} confirmed orders (limit {INITIAL_INVENTORY})")
    else:
        print(f"  FAIL — {len(confirmed)} confirmed orders exceeded limit of {INITIAL_INVENTORY}")

    # ── Check 2: No duplicate order IDs ──────────────────────────────
    print("\n[Check 2] No duplicate order IDs...")
    order_ids  = [r["order_id"] for r in confirmed if r.get("order_id")]
    duplicates = [oid for oid, count in collections.Counter(order_ids).items() if count > 1]
    passed = not duplicates
    passes.append(passed)
    if passed:
        print(f"  PASS — all {len(order_ids)} order IDs are unique")
    else:
        print(f"  FAIL — duplicate order IDs: {duplicates}")

    # ── Check 3: All confirmed orders exist in DB ─────────────────────
    print("\n[Check 3] All confirmed orders exist in orders DB...")
    missing_orders = []
    db_confirmed_count = 0
    try:
        conn = psycopg2.connect(ORDERS_DB_URL)
        cur  = conn.cursor()

        # batch query instead of one per order
        cur.execute("SELECT id FROM orders WHERE status = 'confirmed'")
        db_order_ids = {row[0] for row in cur.fetchall()}
        db_confirmed_count = len(db_order_ids)

        api_order_ids = set(order_ids)
        missing_orders = list(api_order_ids - db_order_ids)

        cur.close()
        conn.close()

        passed = not missing_orders
        passes.append(passed)
        if passed:
            print(f"  PASS — all {len(api_order_ids)} confirmed orders found in DB")
        else:
            print(f"  FAIL — {len(missing_orders)} orders missing from DB: {missing_orders}")
    except Exception as e:
        passes.append(False)
        print(f"  FAIL — could not connect to orders DB: {e}")

    # ── Check 4: Inventory reconciliation ────────────────────────────
    print("\n[Check 4] Inventory reconciliation (confirmed + remaining = initial)...")
    try:
        conn = psycopg2.connect(INVENTORY_DB_URL)
        cur  = conn.cursor()
        cur.execute("SELECT remaining FROM inventory WHERE event_id = %s", (EVENT_ID,))
        row = cur.fetchone()
        cur.close()
        conn.close()

        if row is None:
            passes.append(False)
            print(f"  FAIL — no inventory row found for event {EVENT_ID}")
        else:
            remaining = row[0]
            total = db_confirmed_count + remaining
            passed = total == INITIAL_INVENTORY
            passes.append(passed)
            if passed:
                print(f"  PASS — {db_confirmed_count} confirmed + {remaining} remaining = {INITIAL_INVENTORY}")
            else:
                print(f"  FAIL — {db_confirmed_count} confirmed + {remaining} remaining = {total} (expected {INITIAL_INVENTORY})")
    except Exception as e:
        passes.append(False)
        print(f"  FAIL — could not connect to inventory DB: {e}")

    # ── Check 5: Every confirmed order has a notification ─────────────
    print("\n[Check 5] Every confirmed order has a notification record...")
    try:
        conn = psycopg2.connect(ORDERS_DB_URL)
        cur  = conn.cursor()
        cur.execute("""
            SELECT o.id
            FROM orders o
            LEFT JOIN notifications n ON o.id = n.order_id
            WHERE o.status = 'confirmed' AND n.order_id IS NULL
        """)
        missing_notifications = [row[0] for row in cur.fetchall()]
        cur.close()
        conn.close()

        passed = not missing_notifications
        passes.append(passed)
        if passed:
            print(f"  PASS — all confirmed orders have a notification record")
        else:
            print(f"  FAIL — {len(missing_notifications)} orders missing notifications: {missing_notifications}")
    except Exception as e:
        passes.append(False)
        print(f"  FAIL — could not query notifications: {e}")

    # ── Summary ───────────────────────────────────────────────────────
    print("\n" + "─" * 40)
    if all(passes):
        print("ALL CHECKS PASSED — system is correct")
    else:
        print("SOME CHECKS FAILED — review output above")


if __name__ == "__main__":
    main()