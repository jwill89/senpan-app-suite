using System;
using System.IO;
using System.Runtime.InteropServices;

namespace SenpanCompanion.Services;

/// <summary>
/// Short synthesized tones the plugin plays when something happens the operator
/// should hear without watching the window: a new bingo winner (mirroring the web
/// admin's winner chime) and a tea-room lock running out.
///
/// Each sound is synthesized once into an in-memory WAV and played via the Windows
/// multimedia API (winmm PlaySound, async). This deliberately uses NO game interop,
/// so it can't affect the client or raise an automation concern; it's pure local
/// audio feedback on the operator's PC.
/// </summary>
public static class Chime
{
    private const uint SndAsync = 0x0001;
    private const uint SndMemory = 0x0004;
    private const uint SndNoDefault = 0x0002;

    /// <summary>
    /// Addresses of the PINNED WAV buffers handed to winmm, one per sound.
    ///
    /// SND_ASYNC means PlaySound returns immediately and keeps reading this memory
    /// for the time the tone lasts, but the P/Invoke marshaller only pins a managed
    /// array for the duration of the CALL - so a compacting GC during playback was
    /// free to relocate the buffer out from under winmm, leaving it streaming
    /// whatever now occupies that address. Pinning for the process lifetime is the
    /// fix and costs two small permanently-pinned arrays; the sounds exist for as
    /// long as the plugin does, so there is nothing to free.
    /// </summary>
    private static IntPtr winnerWav;
    private static IntPtr unlockWav;

    [DllImport("winmm.dll", SetLastError = true)]
    private static extern bool PlaySound(IntPtr data, IntPtr hModule, uint flags);

    /// <summary>A rising arpeggio for a new bingo winner. Best-effort - failures are swallowed.</summary>
    public static void Winner() => Play(ref winnerWav, BuildWinner);

    /// <summary>
    /// A two-note fall for a tea-room lock expiring. Deliberately unlike the winner
    /// chime: the two land in the same ear, at unrelated moments, and an operator
    /// should be able to tell "a room freed up" from "somebody won" without looking.
    /// </summary>
    public static void Unlock() => Play(ref unlockWav, BuildUnlock);

    /// <summary>
    /// Plays one sound asynchronously, synthesizing and pinning it on first use.
    /// Every caller reaches this from the framework thread (live events marshal
    /// there first), so the lazy build needs no lock of its own.
    /// </summary>
    private static void Play(ref IntPtr slot, Func<byte[]> build)
    {
        try
        {
            if (slot == IntPtr.Zero)
            {
                // Deliberately never freed: the handle must outlive every async
                // playback, and the plugin unloading takes the whole buffer with it.
                var handle = GCHandle.Alloc(build(), GCHandleType.Pinned);
                slot = handle.AddrOfPinnedObject();
            }
            PlaySound(slot, IntPtr.Zero, SndAsync | SndMemory | SndNoDefault);
        }
        catch
        {
            // Audio is a nicety; never let it disrupt the UI.
        }
    }

    /// <summary>C5 -> E5 -> G5 -> C6, a short celebratory arpeggio.</summary>
    private static readonly double[] WinnerNotes = { 523.25, 659.25, 783.99, 1046.50 };

    /// <summary>G5 -> C5, a short falling pair - a door being unlocked, not a fanfare.</summary>
    private static readonly double[] UnlockNotes = { 783.99, 523.25 };

    private static byte[] BuildWinner() => BuildTones(WinnerNotes, 0.16);

    private static byte[] BuildUnlock() => BuildTones(UnlockNotes, 0.20);

    /// <summary>
    /// Renders a sequence of sine tones, each with a quick exponential decay, into
    /// a mono 16-bit WAV.
    /// </summary>
    private static byte[] BuildTones(double[] freqs, double noteSeconds)
    {
        const int sampleRate = 44100;
        var perNote = (int)(sampleRate * noteSeconds);
        var pcm = new short[perNote * freqs.Length];

        var idx = 0;
        foreach (var f in freqs)
        {
            for (var i = 0; i < perNote; i++)
            {
                var t = i / (double)sampleRate;
                var env = Math.Exp(-t * 7.0); // quick exponential decay
                var sample = Math.Sin(2 * Math.PI * f * t) * env * 0.35;
                pcm[idx++] = (short)(sample * short.MaxValue);
            }
        }

        return WrapWav(pcm, sampleRate);
    }

    private static byte[] WrapWav(short[] pcm, int sampleRate)
    {
        var dataBytes = pcm.Length * 2;
        using var ms = new MemoryStream(44 + dataBytes);
        using var bw = new BinaryWriter(ms);

        bw.Write("RIFF"u8.ToArray());
        bw.Write(36 + dataBytes);
        bw.Write("WAVE"u8.ToArray());
        bw.Write("fmt "u8.ToArray());
        bw.Write(16);             // PCM fmt chunk size
        bw.Write((short)1);       // PCM
        bw.Write((short)1);       // mono
        bw.Write(sampleRate);
        bw.Write(sampleRate * 2); // byte rate (mono, 16-bit)
        bw.Write((short)2);       // block align
        bw.Write((short)16);      // bits per sample
        bw.Write("data"u8.ToArray());
        bw.Write(dataBytes);
        foreach (var s in pcm)
            bw.Write(s);

        bw.Flush();
        return ms.ToArray();
    }
}
