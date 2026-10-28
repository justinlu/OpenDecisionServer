#include <stddef.h>
#include <stdio.h>
#include "llama.h"
int main(void) {
 printf("{\"model_params_size\":%zu,\"context_params_size\":%zu,\"batch_size\":%zu,\"n_ctx_offset\":%zu,\"n_threads_offset\":%zu,\"flash_attn_offset\":%zu,\"offload_kqv_offset\":%zu}\n",sizeof(struct llama_model_params),sizeof(struct llama_context_params),sizeof(struct llama_batch),offsetof(struct llama_context_params,n_ctx),offsetof(struct llama_context_params,n_threads),offsetof(struct llama_context_params,flash_attn_type),offsetof(struct llama_context_params,offload_kqv));
 return 0;
}
