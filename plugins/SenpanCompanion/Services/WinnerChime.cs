using System;
using System.IO;
using System.Runtime.InteropServices;

namespace SenpanCompanion.Services;

/// <summary>
/// Plays a short ascending arpeggio when a new bingo winner appears, so the caller
/// hears it without watching the window - mirroring the web admin's winner chime.
///
/// The sound is synthesized once into an in-memory WAV and played via the Windows
/// multimedia API (winmm PlaySound, async). It deliberately uses NO game interop,
/// so it can't affect the client or raise an automation concern; it's pure local
/// audio feedback on the operator's PC.
/// </summary>
public static class WinnerChime
{
    private const uint SndAsync = 0x0001;
    private const uint SndMemory = 0x0004;
    private const uint SndNoDefault = 0x0002;

    /// <summary>
    /// Address of the PINNED WAV buffer handed to winmm.
    ///
    /// SND_ASYNC means PlaySound returns immediately and keeps reading this memory
    /// for the ~0.64s the chime lasts, but the P/Invoke marshaller only pins a
    /// managed array for the duration of the CALL - so a compacting GC during
    /// playback was free to relocate the ~56KB buffer out from under winmm, leaving
    /// it streaming whatever now occupies that address. Pinning for the process
    /// lifetime is the fix and costs one small permanently-pinned array; the chime
    /// exists for as long as the plugin does, so there is nothing to free.
    /// </summary>
    private static IntPtr wavPtr;

    [DllImport("winmm.dll", SetLastError = true)]
    private static extern bool PlaySound(IntPtr data, IntPtr hModule, uint flags);

    /// <summary>Plays the chime asynchronously. Best-effort - failures are swallowed.</summary>
    public static void Play()
    {
        try
        {
            if (wavPtr == IntPtr.Zero)
            {
                // Deliberately never freed: the handle must outlive every async
                // playback, and the plugin unloading takes the whole buffer with it.
                var handle = GCHandle.Alloc(Build(), GCHandleType.Pinned);
                wavPtr = handle.AddrOfPinnedObject();
            }
            PlaySound(wavPtr, IntPtr.Zero, SndAsync | SndMemory | SndNoDefault);
        }
        catch
        {
            // Audio is a nicety; never let it disrupt the UI.
        }
    }

    private static byte[] Build()
    {
        const int sampleRate = 44100;
        // C5 -> E5 -> G5 -> C6, a short celebratory arpeggio.
        double[] freqs = { 523.25, 659.25, 783.99, 1046.50 };
        const double noteSeconds = 0.16;
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
