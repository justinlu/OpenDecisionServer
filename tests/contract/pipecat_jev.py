"""Run against a ready local ODS: ODS_URL=http://127.0.0.1:8080 python pipecat_jev.py"""

import asyncio
import os

from pipecat.classifiers.base_classifier import ChoiceQuestion, ScoreQuestion, YesNoQuestion
from pipecat.classifiers.jev.classifier import JevClassifier


async def main() -> None:
    classifier = JevClassifier(
        api_key="local", base_url=os.environ.get("ODS_URL", "http://127.0.0.1:8080")
    )
    try:
        await classifier.client.connect()
        answers, usage = await classifier._ask(
            {"message": "Payment failed twice", "customer": "VIP"},
            {
                "urgent": YesNoQuestion(instructions="Does this need attention?"),
                "route": ChoiceQuestion(
                    instructions="Which team should help?",
                    options={"billing": "Payments", "support": "Account help"},
                ),
                "severity": ScoreQuestion(
                    instructions="How urgent is this?",
                    levels=["Can wait", "Soon", "Blocking"],
                ),
            },
        )
        assert 0 <= answers["urgent"].probability <= 1
        assert answers["route"].choice in {"billing", "support"}
        assert 0 <= answers["severity"].score <= 2
        assert usage.prompt_tokens > 0
        print("Pipecat JevClassifier contract passed")
    finally:
        await classifier.client.close()


asyncio.run(main())
