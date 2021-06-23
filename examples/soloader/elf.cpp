#include <stdio.h>  
#include <dlfcn.h>
#include <string.h>
#include "soloader.h"

GoString buildGoString(const char* p, size_t n){
    //typedef struct { const char *p; ptrdiff_t n; } _GoString_;
    //typedef _GoString_ GoString;
    return {p, static_cast<ptrdiff_t>(n)};
}

typedef void (*loaderFunc)(GoString, GoString, GoString);

int main(int argc, char **argv) {  
	void *handle;  
	loaderFunc func;
	char *error;
	const char * objfile = "./soloadertest.o";
	const char * run = "main.main";
	const char * loaderso = "./soloader.so";
  
	handle = dlopen (loaderso, RTLD_LAZY);
	if (!handle) {  
		fprintf (stderr, "%s \n", dlerror());
        return 0;
	}  
  
	func = (loaderFunc)dlsym(handle, "loader");
	if ((error = dlerror()) != NULL)  {  
		fprintf (stderr, "%s \n", error);
		return 0;
	}

	GoString objfileGoStr = buildGoString(objfile, strlen(objfile));
	GoString runGoStr = buildGoString(run, strlen(run));
	GoString loadersoGoStr = buildGoString(loaderso, strlen(loaderso));
	(*func)(objfileGoStr, runGoStr, loadersoGoStr);
	dlclose(handle);  
	return 0;  
}  
