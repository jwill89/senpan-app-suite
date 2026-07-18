using System.Numerics;
using Dalamud.Bindings.ImGui;

namespace SenpanCompanion.Windows;

/// <summary>
/// Small text helpers. ImGui's <c>Text</c> / <c>TextColored</c> / <c>TextDisabled</c> draw a
/// single unwrapped line that runs off the edge of a narrow window; these wrap at the content
/// region's right edge (like <c>TextWrapped</c>) while keeping the colour. They use
/// <c>TextUnformatted</c> under a pushed wrap position rather than <c>TextWrapped</c>, so text
/// that contains a literal <c>%</c> (e.g. a user's macro message) is never treated as a
/// printf format string.
/// </summary>
internal static class UiText
{
    /// <summary>Word-wrapped text in <paramref name="color"/>.</summary>
    public static void WrappedColored(Vector4 color, string text)
    {
        ImGui.PushStyleColor(ImGuiCol.Text, color);
        Wrapped(text);
        ImGui.PopStyleColor();
    }

    /// <summary>Word-wrapped text in the theme's dimmed (disabled) colour.</summary>
    public static void WrappedDisabled(string text)
    {
        ImGui.PushStyleColor(ImGuiCol.Text, ImGui.GetColorU32(ImGuiCol.TextDisabled));
        Wrapped(text);
        ImGui.PopStyleColor();
    }

    /// <summary>Word-wrapped text in the current colour, wrapping at the window content edge.</summary>
    public static void Wrapped(string text)
    {
        ImGui.PushTextWrapPos(0f);
        ImGui.TextUnformatted(text);
        ImGui.PopTextWrapPos();
    }
}
