; =============================================================================
; Excel Translator NSIS 安装脚本
; 用于生成 Windows 安装程序
; =============================================================================

!include "MUI2.nsh"
!include "FileFunc.nsh"

; =============================================================================
; 基本信息
; =============================================================================

!define APP_NAME "Excel Translator"
!define APP_VERSION "1.0.0"
!define APP_PUBLISHER "Excel Translator"
!define APP_EXE "Excel Translator.exe"
!define APP_ICON "icon.ico"
!define SOURCE_DIR "out\windows"
!define OUTPUT_DIR "out"

Name "${APP_NAME}"
OutFile "${OUTPUT_DIR}\${APP_NAME} Setup.exe"
InstallDir "$PROGRAMFILES\${APP_NAME}"
InstallDirRegKey HKLM "Software\${APP_NAME}" "InstallDir"
RequestExecutionLevel admin

; =============================================================================
; 界面设置
; =============================================================================

!define MUI_ABORTWARNING
!define MUI_ICON "${APP_ICON}"
!define MUI_UNICON "${APP_ICON}"

; 欢迎页面
!insertmacro MUI_PAGE_WELCOME

; 许可协议页面（可选，如果有 LICENSE 文件）
; !insertmacro MUI_PAGE_LICENSE "LICENSE"

; 安装目录选择
!insertmacro MUI_PAGE_DIRECTORY

; 安装过程
!insertmacro MUI_PAGE_INSTFILES

; 完成页面
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APP_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "运行 ${APP_NAME}"
!insertmacro MUI_PAGE_FINISH

; 卸载页面
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

; =============================================================================
; 语言设置
; =============================================================================

!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

; =============================================================================
; 安装部分
; =============================================================================

Section "主程序" SecMain
    SectionIn RO
    
    ; 设置输出目录
    SetOutPath "$INSTDIR"
    
    ; 复制主程序和根目录 DLL
    File "${SOURCE_DIR}\${APP_EXE}"
    File "${SOURCE_DIR}\qt.conf"
    File /nonfatal "${SOURCE_DIR}\*.dll"
    
    ; 复制 platforms 插件目录
    SetOutPath "$INSTDIR\platforms"
    File /nonfatal "${SOURCE_DIR}\platforms\*.dll"
    
    ; 复制 styles 插件目录
    SetOutPath "$INSTDIR\styles"
    File /nonfatal "${SOURCE_DIR}\styles\*.dll"
    
    ; 复制 iconengines 插件目录
    SetOutPath "$INSTDIR\iconengines"
    File /nonfatal "${SOURCE_DIR}\iconengines\*.dll"
    
    ; 复制 imageformats 插件目录
    SetOutPath "$INSTDIR\imageformats"
    File /nonfatal "${SOURCE_DIR}\imageformats\*.dll"
    
    ; 返回主安装目录
    SetOutPath "$INSTDIR"
    
    ; 写入卸载信息到注册表
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "DisplayName" "${APP_NAME}"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "UninstallString" '"$INSTDIR\uninstall.exe"'
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "DisplayIcon" "$INSTDIR\${APP_EXE}"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "Publisher" "${APP_PUBLISHER}"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "DisplayVersion" "${APP_VERSION}"
    WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "NoModify" 1
    WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "NoRepair" 1
    
    ; 计算安装大小
    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "EstimatedSize" "$0"
    
    ; 保存安装目录
    WriteRegStr HKLM "Software\${APP_NAME}" "InstallDir" "$INSTDIR"
    
    ; 创建卸载程序
    WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd

Section "开始菜单快捷方式" SecStartMenu
    ; 创建开始菜单文件夹
    CreateDirectory "$SMPROGRAMS\${APP_NAME}"
    
    ; 创建快捷方式
    CreateShortcut "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}" 0
    CreateShortcut "$SMPROGRAMS\${APP_NAME}\卸载 ${APP_NAME}.lnk" "$INSTDIR\uninstall.exe" "" "$INSTDIR\uninstall.exe" 0
SectionEnd

Section "桌面快捷方式" SecDesktop
    CreateShortcut "$DESKTOP\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}" 0
SectionEnd

; =============================================================================
; 卸载部分
; =============================================================================

Section "Uninstall"
    ; 删除注册表项
    DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}"
    DeleteRegKey HKLM "Software\${APP_NAME}"
    
    ; 删除开始菜单快捷方式
    Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
    Delete "$SMPROGRAMS\${APP_NAME}\卸载 ${APP_NAME}.lnk"
    RMDir "$SMPROGRAMS\${APP_NAME}"
    
    ; 删除桌面快捷方式
    Delete "$DESKTOP\${APP_NAME}.lnk"
    
    ; 删除插件目录
    RMDir /r "$INSTDIR\platforms"
    RMDir /r "$INSTDIR\styles"
    RMDir /r "$INSTDIR\iconengines"
    RMDir /r "$INSTDIR\imageformats"
    
    ; 删除主程序和 DLL
    Delete "$INSTDIR\${APP_EXE}"
    Delete "$INSTDIR\qt.conf"
    Delete "$INSTDIR\*.dll"
    Delete "$INSTDIR\uninstall.exe"
    
    ; 删除安装目录（如果为空）
    RMDir "$INSTDIR"
SectionEnd

; =============================================================================
; 区段描述
; =============================================================================

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${SecMain} "安装 ${APP_NAME} 主程序和必要的运行时文件。"
    !insertmacro MUI_DESCRIPTION_TEXT ${SecStartMenu} "在开始菜单中创建快捷方式。"
    !insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} "在桌面创建快捷方式。"
!insertmacro MUI_FUNCTION_DESCRIPTION_END
