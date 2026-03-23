#ifndef MyAppVersion
  #define MyAppVersion "0.0.0"
#endif

#ifndef MyAppPublisher
  #define MyAppPublisher "KeyanHu"
#endif

#ifndef MyAppName
  #define MyAppName "Clash for Claw"
#endif

#ifndef MyPublishDir
  #error MyPublishDir must be defined.
#endif

#ifndef MyOutputDir
  #error MyOutputDir must be defined.
#endif

#ifndef MyOutputBaseFilename
  #define MyOutputBaseFilename "Clash-for-Claw-setup"
#endif

#ifndef MyLicenseFile
  #error MyLicenseFile must be defined.
#endif

#ifndef MyArchitecturesAllowed
  #define MyArchitecturesAllowed ""
#endif

#ifndef MyArchitecturesInstallIn64BitMode
  #define MyArchitecturesInstallIn64BitMode ""
#endif

[Setup]
AppId={{C976D5AE-7458-4AE6-9803-CB4E6F41A9F0}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppVerName={#MyAppName} {#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={localappdata}\Programs\{#MyAppName}
DefaultGroupName={#MyAppName}
LicenseFile={#MyLicenseFile}
OutputDir={#MyOutputDir}
OutputBaseFilename={#MyOutputBaseFilename}
SetupIconFile={#MyPublishDir}\Assets\ClashForClaw.ico
UninstallDisplayIcon={app}\ClashForClaw.exe
WizardStyle=modern
Compression=lzma2/ultra64
SolidCompression=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
ShowLanguageDialog=no
DisableProgramGroupPage=yes
CloseApplications=yes
RestartApplications=no
UsePreviousAppDir=yes
ChangesEnvironment=no
DisableReadyMemo=no
SetupLogging=yes
#if MyArchitecturesAllowed != ""
ArchitecturesAllowed={#MyArchitecturesAllowed}
#endif
#if MyArchitecturesInstallIn64BitMode != ""
ArchitecturesInstallIn64BitMode={#MyArchitecturesInstallIn64BitMode}
#endif

[Languages]
Name: "chinesesimp"; MessagesFile: "compiler:Default.isl,ChineseSimplified.isl"

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "附加选项："; Flags: unchecked
Name: "autostart"; Description: "登录后静默启动"; GroupDescription: "附加选项："; Flags: unchecked

[Files]
Source: "{#MyPublishDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\Clash for Claw"; Filename: "{app}\ClashForClaw.exe"; WorkingDir: "{app}"; IconFilename: "{app}\Assets\ClashForClaw.ico"
Name: "{group}\卸载 Clash for Claw"; Filename: "{uninstallexe}"
Name: "{autodesktop}\Clash for Claw"; Filename: "{app}\ClashForClaw.exe"; WorkingDir: "{app}"; IconFilename: "{app}\Assets\ClashForClaw.ico"; Tasks: desktopicon

[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "ClashForClaw"; ValueData: """{app}\ClashForClaw.exe"" --silent"; Flags: uninsdeletevalue; Tasks: autostart
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: none; ValueName: "OpenClawAdapter"; Flags: deletevalue

[Run]
Filename: "{app}\ClashForClaw.exe"; Description: "启动应用"; Flags: nowait postinstall skipifsilent
