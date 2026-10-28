#ifndef ODS_D1_BRIDGE_H
#define ODS_D1_BRIDGE_H
#ifdef __cplusplus
extern "C" {
#endif
#define ODS_D1_ABI 1
void * ods_d1_open(const char * model, int threads, char ** error);
/* 0 success, 1 invalid request, 2 runtime failure. Strings released by ods_d1_free. */
int ods_d1_run(void * handle, const char * request, int token_budget, char ** output);
char * ods_d1_info(void * handle);
void ods_d1_close(void * handle);
void ods_d1_free(char * text);
int ods_d1_abi(void);
#ifdef __cplusplus
}
#endif
#endif
