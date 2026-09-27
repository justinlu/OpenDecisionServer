"""Run against a ready local ODS: ODS_URL=http://127.0.0.1:8080 python official_sdk.py"""

import os

from typesafe_sdk import Choice, Noul, Score, TypeSafeClient


base_url = os.environ.get("ODS_URL", "http://127.0.0.1:8080")
with TypeSafeClient(api_key="local", base_url=base_url, model="jev-latest") as client:
    result = client.system_one(
        state={"message": "Payment failed twice", "customer": "VIP"},
        questions={
            "urgent": Noul(instructions="Does this need attention?"),
            "route": Choice(
                instructions="Which team should help?",
                criteria={"billing": "Payments", "support": "Account help"},
            ),
            "severity": Score(
                instructions="How urgent is this?",
                criteria=["Can wait", "Soon", "Blocking"],
            ),
        },
    )

assert result.model == "julia-1"
assert 0 <= result.nouls["urgent"].noul <= 1
assert result.choices["route"].choice in {"billing", "support"}
assert 0 <= result.scores["severity"].score <= 2
assert result.usage.input_tokens > 0
print("official TypeSafe SDK contract passed")
