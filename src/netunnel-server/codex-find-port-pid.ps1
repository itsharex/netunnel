$ErrorActionPreference = 'Stop'

$source = @"
using System;
using System.Net;
using System.Runtime.InteropServices;

public static class TcpPortInspector {
    [DllImport("iphlpapi.dll", SetLastError = true)]
    private static extern uint GetExtendedTcpTable(
        IntPtr pTcpTable,
        ref int dwOutBufLen,
        bool sort,
        int ipVersion,
        TCP_TABLE_CLASS tblClass,
        uint reserved
    );

    private enum TCP_TABLE_CLASS {
        TCP_TABLE_OWNER_PID_LISTENER = 3
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct MIB_TCPROW_OWNER_PID {
        public uint state;
        public uint localAddr;
        public uint localPort;
        public uint remoteAddr;
        public uint remotePort;
        public uint owningPid;
    }

    public static int FindListeningPid(int port) {
        int bufferSize = 0;
        GetExtendedTcpTable(IntPtr.Zero, ref bufferSize, true, 2, TCP_TABLE_CLASS.TCP_TABLE_OWNER_PID_LISTENER, 0);
        IntPtr tablePtr = Marshal.AllocHGlobal(bufferSize);
        try {
            uint result = GetExtendedTcpTable(tablePtr, ref bufferSize, true, 2, TCP_TABLE_CLASS.TCP_TABLE_OWNER_PID_LISTENER, 0);
            if (result != 0) {
                return -1;
            }

            int rowCount = Marshal.ReadInt32(tablePtr);
            IntPtr rowPtr = IntPtr.Add(tablePtr, 4);
            int rowSize = Marshal.SizeOf(typeof(MIB_TCPROW_OWNER_PID));

            for (int i = 0; i < rowCount; i++) {
                var row = Marshal.PtrToStructure<MIB_TCPROW_OWNER_PID>(rowPtr);
                int localPort = (int)IPAddress.NetworkToHostOrder((short)((row.localPort >> 16) & 0xFFFF));
                if (localPort == port) {
                    return (int)row.owningPid;
                }
                rowPtr = IntPtr.Add(rowPtr, rowSize);
            }

            return -1;
        } finally {
            Marshal.FreeHGlobal(tablePtr);
        }
    }
}
"@

Add-Type -TypeDefinition $source

$ownerPid = [TcpPortInspector]::FindListeningPid(40061)
Write-Output "PID=$ownerPid"
if ($ownerPid -gt 0) {
  try {
    $proc = Get-Process -Id $ownerPid -ErrorAction Stop
    Write-Output "PROCESS=$($proc.ProcessName)"
    try {
      Write-Output "PATH=$($proc.Path)"
    } catch {
      Write-Output 'PATH_UNAVAILABLE'
    }
  } catch {
    Write-Output 'PROCESS_NOT_FOUND'
  }
}
