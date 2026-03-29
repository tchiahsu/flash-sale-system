from locust import HttpUser, task, between
import random
import string


class FlashSaleTest(HttpUser):
    """Alternative: Burst traffic pattern"""
    wait_time = between(0, 0.1)

    def on_start(self):
        self.user_id = "user-" + "".join(random.choices(string.ascii_lowercase
                                                        + string.digits, k=8))

    @task
    def buy_ticket(self):
        payload = {
            "event_id": "evt-001",
            "user_id": self.user_id,
            "quantity": 1
        }

        self.client.post("/api/orders", json=payload)
