#include "bridge.h"
#include "server-decision.h"
#include "llama.h"
#include "ggml-backend.h"
#include <cmath>
#include <cstring>
#include <cstdlib>
#include <stdexcept>
#include <atomic>

namespace {
char * copy(const std::string & s) {
    auto * p = static_cast<char *>(std::malloc(s.size()+1));
    if (p) std::memcpy(p,s.c_str(),s.size()+1);
    return p;
}
struct runtime {
    llama_model * model = nullptr;
    llama_context * ctx = nullptr;
    bool initialized = false;
    bool logger_installed = false;
    std::atomic<bool> backend_error{false};
    server_decision_context decision;
    uint64_t runs = 0;
    std::string gpu;
    ~runtime() {
        if (ctx) { llama_synchronize(ctx); llama_free(ctx); }
        if (model) llama_model_free(model);
        if (initialized) llama_backend_free();
        if (logger_installed) llama_log_set(nullptr,nullptr);
    }
};
void backend_log(ggml_log_level level,const char * text,void * user) {
    if (level==GGML_LOG_LEVEL_ERROR) static_cast<runtime *>(user)->backend_error.store(true);
    std::fputs(text,stderr);
}
}
extern "C" int ods_d1_abi() { return ODS_D1_ABI; }
extern "C" void ods_d1_free(char * text) { std::free(text); }
extern "C" void ods_d1_close(void * handle) { delete static_cast<runtime *>(handle); }
extern "C" void * ods_d1_open(const char * model, int threads, char ** error) {
    *error = nullptr;
    try {
        auto r = std::make_unique<runtime>();
        llama_log_set(backend_log,r.get());r->logger_installed=true;
        llama_backend_init(); r->initialized = true;
        auto gpu = ggml_backend_dev_by_type(GGML_BACKEND_DEVICE_TYPE_GPU);
        if (!gpu || !llama_supports_gpu_offload()) throw std::runtime_error("Metal GPU unavailable");
        r->gpu = ggml_backend_dev_description(gpu);
        auto mp = llama_model_default_params();
        ggml_backend_dev_t devices[] = {gpu,nullptr}; mp.devices=devices; mp.n_gpu_layers=99;
        r->model = llama_model_load_from_file(model,mp);
        if (!r->model) throw std::runtime_error("d1 model load failed");
        r->decision.init(r->model);
        if (r->decision.type != COMMON_DECISION_TYPE_LFM3_D1) throw std::runtime_error("expected lfm3-d1 model");
        auto cp = llama_context_default_params();
        cp.n_ctx=8192; cp.n_batch=8192; cp.n_ubatch=8192; cp.n_seq_max=1;
        cp.n_threads=threads; cp.n_threads_batch=threads;
        r->ctx=llama_init_from_model(r->model,cp);
        if (!r->ctx || llama_n_ubatch(r->ctx)!=8192 || llama_n_batch(r->ctx)!=8192 || llama_get_causal_attn(r->ctx))
            throw std::runtime_error("d1 noncausal single-batch context mismatch");
        if (r->backend_error.load()) throw std::runtime_error("d1 backend initialization failed; see native error log");
        return r.release();
    } catch (const std::exception & e) { *error=copy(e.what()); }
      catch (...) { *error=copy("unknown d1 initialization failure"); }
    return nullptr;
}
extern "C" char * ods_d1_info(void * handle) {
    try {
        auto & r=*static_cast<runtime *>(handle);
        return copy(json{{"abi",ODS_D1_ABI},{"model","d1-2b"},{"gpu",r.gpu},{"n_ctx",8192},{"n_batch",8192},
            {"n_ubatch",8192},{"n_seq_max",1},{"runs",r.runs},{"media",false},{"causal_attention",llama_get_causal_attn(r.ctx)}}.dump());
    } catch (...) { return nullptr; }
}
extern "C" int ods_d1_run(void * handle,const char * request,int token_budget,char ** output) {
    *output=nullptr;
    auto & r=*static_cast<runtime *>(handle);
    if (r.backend_error.load()) { *output=copy("d1 backend is unavailable after a native error"); return 2; }
    std::vector<server_decision_question> questions;
    std::vector<server_task> tasks;
    try {
        auto body=json::parse(request);
        questions=r.decision.parse_questions(body);
        if (questions.empty() || questions.size()>64) throw std::invalid_argument("requires 1-64 questions");
        auto state=r.decision.prepare_state(body.at("state"));
        size_t total=0;
        for (const auto & q:questions) {
            server_task task(SERVER_TASK_TYPE_DECISION);
            r.decision.fill_task(state,questions,q,0,{},nullptr,mtmd_helper_init_opt_default(),task);
            if (task.tokens.empty() || task.tokens.size()>8192 || task.tokens.has_mtmd)
                throw std::invalid_argument("d1 text prompt must fit one 8192-token physical batch");
            total+=task.tokens.size();
            if (total>static_cast<size_t>(token_budget)) throw std::invalid_argument("request compiled token budget exceeded");
            tasks.push_back(std::move(task));
        }
    } catch (const std::exception & e) { *output=copy(e.what()); return 1; }
      catch (...) { *output=copy("unknown d1 input failure"); return 1; }
    try {
        json results=json::array();
        for (size_t q=0;q<tasks.size();q++) {
            auto & task=tasks[q]; const int n=static_cast<int>(task.tokens.size());
            if (auto memory=llama_get_memory(r.ctx)) llama_memory_clear(memory,true);
            auto batch=llama_batch_init(n,0,1);
            batch.n_tokens=n;
            for (int i=0;i<n;i++) { batch.token[i]=task.tokens[i];batch.pos[i]=i;batch.n_seq_id[i]=1;batch.seq_id[i][0]=0;batch.logits[i]=(i==n-1); }
            int rc=llama_decode(r.ctx,batch);
            llama_batch_free(batch);
            if (rc!=0) throw std::runtime_error("llama_decode failed: "+std::to_string(rc));
            llama_synchronize(r.ctx);
            if (r.backend_error.load()) throw std::runtime_error("d1 backend execution failed; refusing decision output");
            const float * logits=llama_get_logits_ith(r.ctx,n-1);
            if (!logits) throw std::runtime_error("missing final logits");
            std::vector<float> scores;size_t offset=0;
            for (int count:task.decision.label_groups) {
                float best=-INFINITY;
                for (int i=0;i<count;i++) {auto token=task.decision.labels.at(offset++);float value=logits[token];if (!std::isfinite(value)) throw std::runtime_error("nonfinite d1 logit");best=std::max(best,value);}
                scores.push_back(best);
            }
            if (offset!=task.decision.labels.size() || scores.size()!=questions[q].options.size()) throw std::runtime_error("invalid candidate groups");
            results.push_back(json{{"id",questions[q].id},{"answer",r.decision.format_answer(questions[q],{scores},false)},{"input_tokens",n}});
            r.runs++;
        }
        *output=copy(results.dump());
        if (!*output) return 2;
        return 0;
    } catch (const std::exception & e) { *output=copy(e.what()); return 2; }
      catch (...) { *output=copy("unknown d1 runtime failure"); return 2; }
}
