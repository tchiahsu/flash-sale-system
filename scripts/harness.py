import psycopg2
import collections
import time

ORDERS_DB_URL    = "postgresql://flashsale:yourpassword@flash-sale-system-orders.coj6iuswmi2o.us-east-1.rds.amazonaws.com:5432/orders"
INVENTORY_DB_URL = "postgresql://flashsale:yourpassword@flash-sale-system-inventory.coj6iuswmi2o.us-east-1.rds.amazonaws.com:5432/inventory"

INITIAL_INVENTORY = 100
EVENT_ID          = "event-001"


def wait_for_queue_drain():
    print("\nWaiting 5s for RabbitMQ notification queue to drain...")
    time.sleep(5)
    print("  Done waiting")


def main():
    wait_for_queue_drain()

    passes = []
    db_confirmed_count = 0
    db_order_ids = []

    # ── Check 1: No overselling ───────────────────────────────────────
    print("\n[Check 1] No overselling (DB confirmed orders)...")
    try:
        conn = psycopg2.connect(ORDERS_DB_URL)
        cur  = conn.cursor()
        cur.execute("SELECT id FROM orders WHERE status = 'confirmed'")
        db_order_ids = [row[0] for row in cur.fetchall()]
        db_confirmed_count = len(db_order_ids)
        cur.close()
        conn.close()

        passed = db_confirmed_count <= INITIAL_INVENTORY
        passes.append(passed)
        if passed:
            print(f"  PASS — {db_confirmed_count} confirmed orders (limit {INITIAL_INVENTORY})")
        else:
            print(f"  FAIL — {db_confirmed_count} confirmed orders exceeded limit of {INITIAL_INVENTORY}")
    except Exception as e:
        passes.append(False)
        print(f"  FAIL — could not connect to orders DB: {e}")

    # ── Check 2: No duplicate order IDs ──────────────────────────────
    print("\n[Check 2] No duplicate order IDs...")
    duplicates = [oid for oid, count in collections.Counter(db_order_ids).items() if count > 1]
    passed = not duplicates
    passes.append(passed)
    if passed:
        print(f"  PASS — all {len(db_order_ids)} order IDs are unique")
    else:
        print(f"  FAIL — duplicate order IDs: {duplicates}")

    # ── Check 3: All confirmed orders exist in DB ─────────────────────
    print("\n[Check 3] Confirmed orders exist in orders DB...")
    passed = db_confirmed_count > 0
    passes.append(passed)
    if passed:
        print(f"  PASS — {db_confirmed_count} confirmed orders found in DB")
    else:
        print(f"  FAIL — no confirmed orders found in DB")

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
