# TokSuite Chinese attribution

Original dataset: **TokSuite Chinese**, curated by the **R3 Research Team / TokSuite authors**.

- Dataset: https://huggingface.co/datasets/toksuite/toksuite_chinese
- Immutable source card and its licensing statement: https://huggingface.co/datasets/toksuite/toksuite_chinese/blob/f3b76350c0bb4f94d008746f7ab1f3a04642958e/README.md
- Paper: https://arxiv.org/abs/2512.20757
- The authors' dataset card explicitly identifies the dataset license as **MIT**. The repository has no separate LICENSE file at this revision. We preserve the source card as `toksuite-source-card.md`, including its full MIT notice if present; we do not infer data permissions from unrelated code licenses.

This repository converts the 13 configs documented in that card into the ODS request format. Original question, choices, answer, notes and IDs are retained in the `upstream` field. The added Chinese instruction asks the model to select the correct answer/completion. No source answers are relabelled to improve local scores. Extra directories outside the card's config list are not imported. Converted data retains the upstream MIT license; original custom diagnostics follow this repository's license.
