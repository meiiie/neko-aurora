//go:build windows

package providerexec

import (
 "io"
 "os"
 "strings"
 "syscall"
 "unsafe"
)

var witnessFinalPath = syscall.NewLazyDLL("kernel32.dll").NewProc("GetFinalPathNameByHandleW")
func witnessHandle(path string,access uint32)(syscall.Handle,error){p,err:=syscall.UTF16PtrFromString(path);if err!=nil{return 0,ErrSavedAuthority};return syscall.CreateFile(p,access,syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,nil,syscall.OPEN_EXISTING,0x00200000|0x02000000,0)}
func privateDirectoryIdentity(path string)bool{h,err:=witnessHandle(path,0);if err!=nil{return false};defer syscall.CloseHandle(h);var info syscall.ByHandleFileInformation;return syscall.GetFileInformationByHandle(h,&info)==nil&&info.FileAttributes&0x400==0&&info.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY!=0}
func readPrivateWitness(path string)([]byte,error){
 if _,err:=privateWitnessPath(path);err!=nil{return nil,err};h,err:=witnessHandle(path,syscall.GENERIC_READ);if err!=nil{return nil,err};f:=os.NewFile(uintptr(h),path);defer f.Close()
 var before,after,named syscall.ByHandleFileInformation;if syscall.GetFileInformationByHandle(h,&before)!=nil||before.NumberOfLinks!=1||before.FileAttributes&(0x400|syscall.FILE_ATTRIBUTE_DIRECTORY)!=0||before.FileSizeHigh!=0||before.FileSizeLow>4096{return nil,ErrSavedAuthority}
 buffer:=make([]uint16,4096);n,_,_:=witnessFinalPath.Call(uintptr(h),uintptr(unsafe.Pointer(&buffer[0])),uintptr(len(buffer)),0);if n==0||n>=uintptr(len(buffer)){return nil,ErrSavedAuthority};resolved:=strings.TrimPrefix(syscall.UTF16ToString(buffer[:n]),`\\?\`);if !strings.EqualFold(resolved,path){return nil,ErrSavedAuthority}
 raw,err:=io.ReadAll(io.LimitReader(f,4097));if err!=nil||len(raw)>4096||syscall.GetFileInformationByHandle(h,&after)!=nil||before!=after{return nil,ErrSavedAuthority}
 second,err:=witnessHandle(path,0);if err!=nil{return nil,ErrSavedAuthority};defer syscall.CloseHandle(second);if syscall.GetFileInformationByHandle(second,&named)!=nil||before!=named{return nil,ErrSavedAuthority};return raw,nil
}
