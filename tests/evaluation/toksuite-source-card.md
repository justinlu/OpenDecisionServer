---
license: mit
multilinguality: multilingual
task_categories:
- multiple-choice
pretty_name: Tokenization Robustness
tags:
- multilingual
- tokenization
- robustness
dataset_info:
- config_name: toksuite_chinese_canonical
  features:
  - name: question
    dtype: string
  - name: choices
    list: string
  - name: answer
    dtype: int64
  - name: answer_label
    dtype: string
  - name: split
    dtype: string
  - name: subcategories
    dtype: string
  - name: category
    dtype: string
  - name: lang
    dtype: string
  - name: second_lang
    dtype: string
  - name: notes
    dtype: string
  - name: id
    dtype: string
  - name: set_id
    dtype: string
  - name: variation_id
    dtype: string
  splits:
  - name: test
    num_bytes: 8225
    num_examples: 40
  download_size: 9396
  dataset_size: 8225
- config_name: toksuite_chinese_code_language_script_switching
  features:
  - name: question
    dtype: string
  - name: choices
    list: string
  - name: answer
    dtype: int64
  - name: answer_label
    dtype: string
  - name: split
    dtype: string
  - name: subcategories
    dtype: string
  - name: category
    dtype: string
  - name: lang
    dtype: string
  - name: second_lang
    dtype: string
  - name: notes
    dtype: string
  - name: id
    dtype: string
  - name: set_id
    dtype: string
  - name: variation_id
    dtype: string
  splits:
  - name: test
    num_bytes: 8136
    num_examples: 40
  download_size: 8261
  dataset_size: 8136
- config_name: toksuite_chinese_colloquial
  features:
  - name: question
    dtype: string
  - name: choices
    list: string
  - name: answer
    dtype: int64
  - name: answer_label
    dtype: string
  - name: split
    dtype: string
  - name: subcategories
    dtype: string
  - name: category
    dtype: string
  - name: lang
    dtype: string
  - name: second_lang
    dtype: string
  - name: notes
    dtype: string
  - name: id
    dtype: string
  - name: set_id
    dtype: string
  - name: variation_id
    dtype: string
  splits:
  - name: test
    num_bytes: 7442
    num_examples: 39
  download_size: 8111
  dataset_size: 7442
- config_name: toksuite_chinese_equivalent_expressions
  features:
  - name: question
    dtype: string
  - name: choices
    list: string
  - name: answer
    dtype: int64
  - name: answer_label
    dtype: string
  - name: split
    dtype: string
  - name: subcategories
    dtype: string
  - name: category
    dtype: string
  - name: lang
    dtype: string
  - name: second_lang
    dtype: string
  - name: notes
    dtype: string
  - name: id
    dtype: string
  - name: set_id
    dtype: string
  - name: variation_id
    dtype: string
  splits:
  - name: test
    num_bytes: 7907
    num_examples: 40
  download_size: 8383
  dataset_size: 7907
- config_name: toksuite_chinese_keyboard_proximity_errors
  features:
  - name: question
    dtype: string
  - name: choices
    list: string
  - name: answer
    dtype: int64
  - name: answer_label
    dtype: string
  - name: split
    dtype: string
  - name: subcategories
    dtype: string
  - name: category
    dtype: string
  - name: lang
    dtype: string
  - name: second_lang
    dtype: string
  - name: notes
    dtype: string
  - name: id
    dtype: string
  - name: set_id
    dtype: string
  - name: variation_id
    dtype: string
  splits:
  - name: test
    num_bytes: 7340
    num_examples: 40
  download_size: 8251
  dataset_size: 7340
- config_name: toksuite_chinese_ocr_errors
  features:
  - name: question
    dtype: string
  - name: choices
    list: string
  - name: answer
    dtype: int64
  - name: answer_label
    dtype: string
  - name: split
    dtype: string
  - name: subcategories
    dtype: string
  - name: category
    dtype: string
  - name: lang
    dtype: string
  - name: second_lang
    dtype: string
  - name: notes
    dtype: string
  - name: id
    dtype: string
  - name: set_id
    dtype: string
  - name: variation_id
    dtype: string
  splits:
  - name: test
    num_bytes: 8441
    num_examples: 40
  download_size: 8307
  dataset_size: 8441
- config_name: toksuite_chinese_optional_diacritics
  features:
  - name: question
    dtype: string
  - name: choices
    list: string
  - name: answer
    dtype: int64
  - name: answer_label
    dtype: string
  - name: split
    dtype: string
  - name: subcategories
    dtype: string
  - name: category
    dtype: string
  - name: lang
    dtype: string
  - name: second_lang
    dtype: string
  - name: notes
    dtype: string
  - name: id
    dtype: string
  - name: set_id
    dtype: string
  - name: variation_id
    dtype: string
  splits:
  - name: test
    num_bytes: 10200
    num_examples: 40
  download_size: 8835
  dataset_size: 10200
- config_name: toksuite_chinese_partially_romanized
  features:
  - name: question
    dtype: string
  - name: choices
    list: string
  - name: answer
    dtype: int64
  - name: answer_label
    dtype: string
  - name: split
    dtype: string
  - name: subcategories
    dtype: string
  - name: category
    dtype: string
  - name: lang
    dtype: string
  - name: second_lang
    dtype: string
  - name: notes
    dtype: string
  - name: id
    dtype: string
  - name: set_id
    dtype: string
  - name: variation_id
    dtype: string
  splits:
  - name: test
    num_bytes: 7680
    num_examples: 40
  download_size: 8217
  dataset_size: 7680
- config_name: toksuite_chinese_romanization
  features:
  - name: question
    dtype: string
  - name: choices
    list: string
  - name: answer
    dtype: int64
  - name: answer_label
    dtype: string
  - name: split
    dtype: string
  - name: subcategories
    dtype: string
  - name: category
    dtype: string
  - name: lang
    dtype: string
  - name: second_lang
    dtype: string
  - name: notes
    dtype: string
  - name: id
    dtype: string
  - name: set_id
    dtype: string
  - name: variation_id
    dtype: string
  splits:
  - name: test
    num_bytes: 7859
    num_examples: 40
  download_size: 8285
  dataset_size: 7859
- config_name: toksuite_chinese_space_removal
  features:
  - name: question
    dtype: string
  - name: choices
    list: string
  - name: answer
    dtype: int64
  - name: answer_label
    dtype: string
  - name: split
    dtype: string
  - name: subcategories
    dtype: string
  - name: category
    dtype: string
  - name: lang
    dtype: string
  - name: second_lang
    dtype: string
  - name: notes
    dtype: string
  - name: id
    dtype: string
  - name: set_id
    dtype: string
  - name: variation_id
    dtype: string
  splits:
  - name: test
    num_bytes: 10554
    num_examples: 40
  download_size: 8618
  dataset_size: 10554
- config_name: toksuite_chinese_spelled_out
  features:
  - name: question
    dtype: string
  - name: choices
    list: string
  - name: answer
    dtype: int64
  - name: answer_label
    dtype: string
  - name: split
    dtype: string
  - name: subcategories
    dtype: string
  - name: category
    dtype: string
  - name: lang
    dtype: string
  - name: second_lang
    dtype: string
  - name: notes
    dtype: string
  - name: id
    dtype: string
  - name: set_id
    dtype: string
  - name: variation_id
    dtype: string
  splits:
  - name: test
    num_bytes: 2583
    num_examples: 13
  download_size: 6308
  dataset_size: 2583
- config_name: toksuite_chinese_traditional
  features:
  - name: question
    dtype: string
  - name: choices
    list: string
  - name: answer
    dtype: int64
  - name: answer_label
    dtype: string
  - name: split
    dtype: string
  - name: subcategories
    dtype: string
  - name: category
    dtype: string
  - name: lang
    dtype: string
  - name: second_lang
    dtype: string
  - name: notes
    dtype: string
  - name: id
    dtype: string
  - name: set_id
    dtype: string
  - name: variation_id
    dtype: string
  splits:
  - name: test
    num_bytes: 6125
    num_examples: 33
  download_size: 7768
  dataset_size: 6125
- config_name: >-
    toksuite_chinese_word_spacing_zero-width_characters_extra_space
  features:
  - name: question
    dtype: string
  - name: choices
    list: string
  - name: answer
    dtype: int64
  - name: answer_label
    dtype: string
  - name: split
    dtype: string
  - name: subcategories
    dtype: string
  - name: category
    dtype: string
  - name: lang
    dtype: string
  - name: second_lang
    dtype: string
  - name: notes
    dtype: string
  - name: id
    dtype: string
  - name: set_id
    dtype: string
  - name: variation_id
    dtype: string
  splits:
  - name: test
    num_bytes: 8831
    num_examples: 40
  download_size: 8368
  dataset_size: 8831
configs:
- config_name: toksuite_chinese_canonical
  data_files:
  - split: test
    path: toksuite_chinese_canonical/test-*
- config_name: toksuite_chinese_code_language_script_switching
  data_files:
  - split: test
    path: >-
      toksuite_chinese_code_language_script_switching/test-*
- config_name: toksuite_chinese_colloquial
  data_files:
  - split: test
    path: toksuite_chinese_colloquial/test-*
- config_name: toksuite_chinese_equivalent_expressions
  data_files:
  - split: test
    path: toksuite_chinese_equivalent_expressions/test-*
- config_name: toksuite_chinese_keyboard_proximity_errors
  data_files:
  - split: test
    path: toksuite_chinese_keyboard_proximity_errors/test-*
- config_name: toksuite_chinese_ocr_errors
  data_files:
  - split: test
    path: toksuite_chinese_ocr_errors/test-*
- config_name: toksuite_chinese_optional_diacritics
  data_files:
  - split: test
    path: toksuite_chinese_optional_diacritics/test-*
- config_name: toksuite_chinese_partially_romanized
  data_files:
  - split: test
    path: toksuite_chinese_partially_romanized/test-*
- config_name: toksuite_chinese_romanization
  data_files:
  - split: test
    path: toksuite_chinese_romanization/test-*
- config_name: toksuite_chinese_space_removal
  data_files:
  - split: test
    path: toksuite_chinese_space_removal/test-*
- config_name: toksuite_chinese_spelled_out
  data_files:
  - split: test
    path: toksuite_chinese_spelled_out/test-*
- config_name: toksuite_chinese_traditional
  data_files:
  - split: test
    path: toksuite_chinese_traditional/test-*
- config_name: >-
    toksuite_chinese_word_spacing_zero-width_characters_extra_space
  data_files:
  - split: test
    path: >-
      toksuite_chinese_word_spacing_zero-width_characters_extra_space/test-*
language:
- en
- zh
size_categories:
- n<1K
---
# Dataset Card for Tokenization Robustness

<!-- Provide a quick summary of the dataset. -->

<img src="toksuite-logo.png" alt="TokSuite Logo" width="250px" style="margin-left:'auto' margin-right:'auto' display:'block'"/>

# TokSuite Benchmark (Chinese Collection)

## Dataset Description

This dataset is part of **TokSuite**, a comprehensive benchmark designed to measure how different tokenization strategies affect language model performance and robustness. This specific subset contains Chinese language multiple-choice text completion questions with various real-world perturbations that test tokenizer robustness.

- **Curated by:** R3 Research Team
- **Language(s):** Chinese (It)
- **License:** MIT License

### Dataset Summary

TokSuite addresses a fundamental challenge in language model research: understanding how tokenization choices impact model behavior in isolation. The Chinese subset specifically measures model performance on canonical questions and various perturbations.

**Key Features:**
- 40 canonical questions covering general knowledge, geography, science, and language understanding
- Multiple perturbation types reflecting real-world text variations in Chinese
- Parallel structure with TokSuite benchmark (available in English, Turkish, Farsi, Italian)
- Native speaker curation ensuring linguistic authenticity

### Supported Tasks

- **Multiple-Choice Question Answering**: Text completion format with 4 answer choices
- **Tokenizer Robustness Evaluation**: Measuring performance degradation under various text perturbations
- **Multilingual NLP Benchmarking**: Evaluating language models on Chinese text understanding

### Languages

The dataset contains text in Chinese (language code: `zho_Hans` / `zh`).

## Dataset Structure

### Data Fields

| Field | Type | Description |
|-------|------|-------------|
| `question` | `string` | The question text in Chinese |
| `choices` | `list[string]` | 4 multiple-choice answer options |
| `answer` | `int64` | Index of the correct answer |
| `answer_label` | `string` | Letter label of the correct answer |
| `split` | `string` | Dataset split identifier |
| `subcategories` | `string` | Perturbation category |
| `lang` | `string` | Language code |
| `second_lang` | `string` | English translation or description of the question |
| `notes` | `string` | Additional context about the question or perturbation |
| `id` | `string` | Unique question identifier |
| `set_id` | `float64` | Question set grouping identifier |
| `variation_id` | `float64` | Variation number within a question set |
| `vanilla_cos_sim_to_canonical` | `dict[string, float]` | Cosine similarity scores to canonical form (raw tokens) |
| `trimmed_cos_sim_to_canonical` | `dict[string, float]` | Cosine similarity scores after token normalization |
| `token_counts` | `dict[string, integer]` | Number of tokens produced per tokenizer |

## Dataset Creation

### Curation Rationale

This dataset was created to:
1. Systematically evaluate how different tokenization strategies handle Chinese
2. Measure robustness against real-world text perturbations specific to Chinese
3. Support research into the impact of tokenization on language model behavior
4. Provide standardized benchmarks for Chinese language models

The questions were designed to be straightforward with high baseline accuracy, allowing researchers to cleanly measure performance degradation when perturbations are applied.

### Source Data

#### Data Collection and Processing

- **Canonical Questions**: 40 baseline questions created in English
- **Translation**: Native Chinese speakers translated questions
- **Perturbations**: Each question underwent targeted perturbations designed to reflect Chinese characteristics
- **Validation**: Model-in-the-loop process ensured high baseline accuracy

#### Perturbation Categories

1. **Canonical**
The baseline Chinese text written in standard, well-formed Simplified Chinese with no perturbations. This serves as the reference condition for evaluating the impact of all other perturbations.

2. **Code / Language / Script Switching**
Mixes Chinese with English words, phrases, or symbols within the same sentence, reflecting real-world bilingual usage and code-switching commonly seen in technical or online contexts.

3. **Colloquial**
Rewrites sentences using informal or conversational Chinese expressions, including spoken-style phrasing that differs from standard written Chinese while preserving meaning.

4. **Equivalent Expressions**
Replaces canonical phrases with alternative Chinese expressions that convey the same meaning using different words or constructions, isolating tokenizer sensitivity to paraphrasing.

5. **Keyboard Proximity Errors**
Introduces character-level errors caused by adjacent key presses in pinyin-based input methods, simulating realistic typing mistakes during Chinese text entry.

6. **OCR Errors**
Introduces character substitutions, deletions, or confusions commonly produced by optical character recognition systems, especially for visually similar Chinese characters.

7. **Optional Diacritics**
Adds or removes optional diacritic markers (e.g., tone marks in pinyin annotations when present), testing tokenizer robustness to auxiliary pronunciation cues.

8. **Partially Romanized**
Mixes Chinese characters with romanized (pinyin or Latin-script) representations for some words or phrases, reflecting hybrid writing styles used in informal digital text.

9. **Romanization**
Fully converts Chinese text into romanized form (e.g., pinyin), replacing characters with Latin-script equivalents while preserving pronunciation and meaning.

10. **Space Removal**
Removes spaces that may appear between Chinese characters or between Chinese and Latin text, stressing tokenizer assumptions about whitespace usage.

11. **Spelled-Out Forms**
Replaces numerals, symbols, or compact expressions with fully spelled-out Chinese equivalents, increasing sequence length and altering token boundaries.

12. **Traditional**
Converts Simplified Chinese characters into their Traditional Chinese counterparts, preserving semantics while changing Unicode character forms.

13. **Word Spacing, Zero-Width Characters, Extra Space**
Manipulates spacing by inserting extra spaces, removing expected spaces, or adding invisible zero-width characters, stressing tokenizer handling of segmentation and Unicode normalization.

#### Who are the source data producers?

Native Chinese speakers curated and validated all questions and perturbations. The TokSuite research team at R3 designed the overall benchmark framework.

### Annotations

#### Annotation process

Questions were manually created and translated by native speakers. Each perturbation was carefully designed to reflect authentic variations encountered in real-world Chinese text processing.

#### Who are the annotators?

Native Chinese speakers with expertise in linguistics and NLP, working as part of the TokSuite project.

### Personal and Sensitive Information

The dataset contains only general knowledge questions and does not include any personal or sensitive information.

## Considerations for Using the Data

### Social Impact of Dataset

This dataset contributes to improving language technology for Chinese speakers by enabling better understanding of tokenization challenges and supporting more robust multilingual models.

### Discussion of Biases

- **Language variety** The dataset uses Standard Chinese (Mandarin) and may not fully represent regional or dialectal variations.
- **Script focus:** Simplified Chinese is used as the primary script; Traditional Chinese and romanized forms (pinyin) are included as perturbations.
- **Domain coverage:** Questions focus on general knowledge and may not represent domain-specific Chinese language use.
- **Question simplicity:** Designed for high baseline accuracy, which may not reflect real-world task complexity.

### Other Known Limitations

- Relatively small dataset size (evaluation-only)
- Multiple-choice format
- Language-specific perturbations
- Results may differ at larger model scales

## Additional Information

### Dataset Curators

The dataset was curated by the TokSuite research team at R3.

### Licensing Information

MIT license 

### Citation Information

If you use this dataset in your research, please cite the TokSuite paper:

```bibtex
@inproceedings{toksuite2026,
  title={TokSuite: Measuring the Impact of Tokenizer Choice on Language Model Behavior},
  author={Altıntaş, Gül Sena and Ehghaghi, Malikeh and Lester, Brian and Liu, Fengyuan and Zhao, Wanru and Ciccone, Marco and Raffel, Colin},
  booktitle={Preprint.},
  year={2026},
  arxiv={https://arxiv.org/abs/2512.20757},
  url={TBD}
}
```

**Paper**: [TokSuite: Measuring the Impact of Tokenizer Choice on Language Model Behavior](TBD)

### Contributions

This dataset is part of TokSuite, which includes:
- 14 language models with identical architectures but different tokenizers
- Multilingual benchmark datasets (English, Turkish, Italian, Farsi, Chinese)
- Comprehensive analysis of tokenization's impact on model behavior


### Contact

For questions or issues related to this dataset, please refer to the TokSuite project or contact the authors of the paper.

---

<div align="center">
  
**Part of the [TokSuite Project](TBD)**

*Understanding Tokenization's Role in Language Model Behavior*

</div>