package cairo

// extern int writeFunc(void *closure, void*data, unsigned int length);
// extern int readFunc(void *closure, void*data, unsigned int length);
// int writeFuncBound(void *closure, void*data, unsigned int length) {
//	 return writeFunc(closure, data, length);
// }
// int readFuncBound(void *closure, void*data, unsigned int length) {
//	 return readFunc(closure, data, length);
// }
import "C"
