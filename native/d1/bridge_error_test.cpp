// Include the implementation to exercise the internal backend-error callback
// without allocating a model or deliberately exhausting the GPU.
#include "bridge.cpp"
int main() {
    runtime r;
    backend_log(GGML_LOG_LEVEL_ERROR,"injected backend command-buffer failure\n",&r);
    char * output=nullptr;
    int status=ods_d1_run(&r,"{}",65536,&output);
    if (!(status==2 && output && std::strstr(output,"backend"))) return 1;
    ods_d1_free(output);
    return 0;
}
