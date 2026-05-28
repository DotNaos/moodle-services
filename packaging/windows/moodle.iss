#define MyAppName "moodle-services"

#ifndef AppVersion
  #error AppVersion must be defined
#endif

#ifndef SourceDir
  #error SourceDir must be defined
#endif

#ifndef OutputDir
  #error OutputDir must be defined
#endif

#ifndef InstallerArch
  #error InstallerArch must be defined
#endif

#if InstallerArch == "amd64"
  #define OutputBase "moodle_windows_amd64_setup"
  #define ArchitecturesAllowedValue "x64compatible"
  #define ArchitecturesInstallMode "x64compatible"
#elif InstallerArch == "arm64"
  #define OutputBase "moodle_windows_arm64_setup"
  #define ArchitecturesAllowedValue "arm64"
  #define ArchitecturesInstallMode "arm64"
#else
  #error Unsupported InstallerArch value
#endif

[Setup]
AppId={{7A7A1A5A-4A5E-4B64-B1B2-13C939D7A111}}
AppName={#MyAppName}
AppVersion={#AppVersion}
AppPublisher=DotNaos
DefaultDirName={localappdata}\Programs\moodle-services
DefaultGroupName=moodle-services
DisableProgramGroupPage=yes
OutputDir={#OutputDir}
OutputBaseFilename={#OutputBase}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=lowest
ArchitecturesAllowed={#ArchitecturesAllowedValue}
ArchitecturesInstallIn64BitMode={#ArchitecturesInstallMode}
ChangesEnvironment=yes
UninstallDisplayIcon={app}\moodle.exe

[Files]
Source: "{#SourceDir}\moodle.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\moodle-services"; Filename: "{app}\moodle.exe"

[Code]
function NeedsAddPath(Path: string): Boolean;
var
  Paths: string;
begin
  if not RegQueryStringValue(HKCU, 'Environment', 'Path', Paths) then
  begin
    Result := True;
    exit;
  end;

  Paths := ';' + Lowercase(Paths) + ';';
  Result := Pos(';' + Lowercase(Path) + ';', Paths) = 0;
end;

procedure AddToUserPath(Path: string);
var
  Paths: string;
begin
  if not RegQueryStringValue(HKCU, 'Environment', 'Path', Paths) then
    Paths := '';

  if (Paths <> '') and (Copy(Paths, Length(Paths), 1) <> ';') then
    Paths := Paths + ';';

  if NeedsAddPath(Path) then
    RegWriteExpandStringValue(HKCU, 'Environment', 'Path', Paths + Path);
end;

procedure RemoveFromUserPath(Path: string);
var
  Paths: string;
  Updated: string;
begin
  if not RegQueryStringValue(HKCU, 'Environment', 'Path', Paths) then
    exit;

  Updated := ';' + Paths + ';';
  StringChangeEx(Updated, ';' + Path + ';', ';', True);
  while Pos(';;', Updated) > 0 do
    StringChangeEx(Updated, ';;', ';', True);

  if (Length(Updated) > 0) and (Copy(Updated, 1, 1) = ';') then
    Delete(Updated, 1, 1);
  if (Length(Updated) > 0) and (Copy(Updated, Length(Updated), 1) = ';') then
    Delete(Updated, Length(Updated), 1);

  RegWriteExpandStringValue(HKCU, 'Environment', 'Path', Updated);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
    AddToUserPath(ExpandConstant('{app}'));
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then
    RemoveFromUserPath(ExpandConstant('{app}'));
end;
