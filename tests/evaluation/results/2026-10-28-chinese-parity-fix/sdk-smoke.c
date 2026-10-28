#include "llama.h"
#include "mtmd.h"
#include "mtmd-helper.h"
int main(void) { return llama_context_default_params().n_batch == 0; }
