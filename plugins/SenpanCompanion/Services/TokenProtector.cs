using System;
using System.Runtime.InteropServices;
using System.Text;

namespace SenpanCompanion.Services;

/// <summary>
/// Encrypts the personal access token at rest using the Windows Data Protection
/// API (DPAPI, <c>CurrentUser</c> scope). The ciphertext is decryptable only by the
/// same Windows user on the same machine, so a synced/backed-up or world-readable
/// plugin config no longer exposes a usable token. Best-effort: if DPAPI is
/// unavailable the methods return null and the caller keeps the token in memory
/// for the session only. The plugin is Windows-only (FFXIV/Dalamud), so DPAPI is
/// always the right primitive here.
/// </summary>
internal static class TokenProtector
{
    // CRYPTPROTECT_UI_FORBIDDEN — never show a UI prompt (we run headless of any dialog).
    private const int UiForbidden = 0x1;

    /// <summary>
    /// Encrypts <paramref name="plaintext"/> and returns base64 ciphertext, or null
    /// when the input is empty or DPAPI fails.
    /// </summary>
    public static string? Protect(string plaintext)
    {
        if (string.IsNullOrEmpty(plaintext))
        {
            return null;
        }

        try
        {
            var cipher = Transform(Encoding.UTF8.GetBytes(plaintext), protect: true);
            return cipher is null ? null : Convert.ToBase64String(cipher);
        }
        catch
        {
            return null;
        }
    }

    /// <summary>
    /// Decrypts base64 ciphertext produced by <see cref="Protect"/>, or returns null
    /// when the input is empty, not valid base64, or DPAPI fails (e.g. copied from a
    /// different machine/user).
    /// </summary>
    public static string? Unprotect(string? protectedBase64)
    {
        if (string.IsNullOrEmpty(protectedBase64))
        {
            return null;
        }

        try
        {
            var plain = Transform(Convert.FromBase64String(protectedBase64), protect: false);
            return plain is null ? null : Encoding.UTF8.GetString(plain);
        }
        catch
        {
            return null;
        }
    }

    private static byte[]? Transform(byte[] input, bool protect)
    {
        var inBlob = default(DataBlob);
        var outBlob = default(DataBlob);
        var pinned = GCHandle.Alloc(input, GCHandleType.Pinned);
        try
        {
            inBlob.CbData = input.Length;
            inBlob.PbData = pinned.AddrOfPinnedObject();

            var ok = protect
                ? CryptProtectData(ref inBlob, null, IntPtr.Zero, IntPtr.Zero, IntPtr.Zero, UiForbidden, ref outBlob)
                : CryptUnprotectData(ref inBlob, IntPtr.Zero, IntPtr.Zero, IntPtr.Zero, IntPtr.Zero, UiForbidden, ref outBlob);
            if (!ok)
            {
                return null;
            }

            var result = new byte[outBlob.CbData];
            Marshal.Copy(outBlob.PbData, result, 0, outBlob.CbData);
            return result;
        }
        finally
        {
            pinned.Free();
            if (outBlob.PbData != IntPtr.Zero)
            {
                _ = LocalFree(outBlob.PbData);
            }
        }
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct DataBlob
    {
        public int CbData;
        public IntPtr PbData;
    }

    [DllImport("crypt32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool CryptProtectData(ref DataBlob dataIn, string? dataDescr, IntPtr optionalEntropy, IntPtr reserved, IntPtr promptStruct, int flags, ref DataBlob dataOut);

    [DllImport("crypt32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool CryptUnprotectData(ref DataBlob dataIn, IntPtr dataDescr, IntPtr optionalEntropy, IntPtr reserved, IntPtr promptStruct, int flags, ref DataBlob dataOut);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern IntPtr LocalFree(IntPtr mem);
}
