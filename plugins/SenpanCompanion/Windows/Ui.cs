using System;
using System.Numerics;
using Dalamud.Bindings.ImGui;
using Dalamud.Interface;
using Dalamud.Interface.Components;
using Dalamud.Interface.Utility.Raii;

namespace SenpanCompanion.Windows;

/// <summary>
/// Shared ImGui building blocks so every page speaks one visual language: icon +
/// accent section headers, a three-tier button system (primary / secondary / danger),
/// compact icon buttons for table rows, pill badges, and a rounded "card" container.
/// Modelled on the sibling TarotGen plugin's Ui helper. Icons come from Dalamud's
/// bundled FontAwesome font; scoped push/pop is done with <see cref="ImRaii"/> so a
/// draw path can never leak a style onto the rest of the frame.
/// </summary>
internal static class Ui
{
    // ── palette ────────────────────────────────────────────────────────────────
    // Mirrors the web admin dashboard's default dark theme
    // (frontend/src/assets/styles/tokens.css) so the plugin reads as the same product.
    public static readonly Vector4 SectionColor = Rgb(0xD6, 0xBD, 0xAE);         // --highlight (warm tan)
    public static readonly Vector4 AccentColor = Rgb(0xD6, 0xBD, 0xAE);          // --accent
    public static readonly Vector4 SuccessColor = new(0.32f, 0.78f, 0.40f, 1f);  // green, kept bright enough for text
    public static readonly Vector4 WarnColor = Rgb(0xE0, 0xA8, 0x2E);            // --warning (amber)
    public static readonly Vector4 InfoColor = new(0.55f, 0.75f, 1f, 1f);        // blue: informational / done

    // Primary CTA = the web --accent (tan), which needs dark text (--text-on-accent).
    private static readonly Vector4 Primary = Rgb(0xD6, 0xBD, 0xAE);             // --accent
    private static readonly Vector4 PrimaryHover = Rgb(0xC4, 0xA9, 0x99);        // --accent-hover
    private static readonly Vector4 PrimaryActive = Rgb(0xB3, 0x98, 0x88);       // darker still
    private static readonly Vector4 PrimaryText = Rgb(0x1A, 0x1C, 0x17);         // --text-on-accent

    // Secondary = the web --accent-2 (dark olive), light text.
    private static readonly Vector4 Secondary = Rgb(0x47, 0x4B, 0x3C);           // --accent-2
    private static readonly Vector4 SecondaryHover = Rgb(0x56, 0x5A, 0x48);
    private static readonly Vector4 SecondaryActive = Rgb(0x3A, 0x3D, 0x30);     // --accent-2-hover

    private static readonly Vector4 Danger = Rgb(0x9A, 0x20, 0x18);              // --danger
    private static readonly Vector4 DangerHover = Rgb(0xB5, 0x2A, 0x20);
    private static readonly Vector4 DangerActive = Rgb(0x7E, 0x1A, 0x13);

    // Card container: a faint raised fill + the web control-border outline.
    private static readonly Vector4 BoxBg = new(1f, 1f, 1f, 0.03f);
    private static readonly Vector4 BoxBorder = Rgb(0x4A, 0x4D, 0x3F);           // --control-border

    private static Vector4 Rgb(byte r, byte g, byte b) => new(r / 255f, g / 255f, b / 255f, 1f);

    // ── icons ──────────────────────────────────────────────────────────────────

    /// <summary>Render a FontAwesome glyph inline (default text colour).</summary>
    public static void Icon(FontAwesomeIcon icon)
    {
        using (ImRaii.PushFont(UiBuilder.IconFont))
            ImGui.TextUnformatted(icon.ToIconString());
    }

    /// <summary>Render a FontAwesome glyph inline in <paramref name="color"/>.</summary>
    public static void Icon(FontAwesomeIcon icon, Vector4 color)
    {
        using (ImRaii.PushColor(ImGuiCol.Text, color))
        using (ImRaii.PushFont(UiBuilder.IconFont))
            ImGui.TextUnformatted(icon.ToIconString());
    }

    // ── section headers ────────────────────────────────────────────────────────

    /// <summary>A non-collapsible section header: icon + gold label + rule.</summary>
    public static void Section(FontAwesomeIcon icon, string label)
    {
        ImGui.Spacing();
        Icon(icon, SectionColor);
        ImGui.SameLine(0, ImGui.GetStyle().ItemInnerSpacing.X * 1.5f);
        ImGui.TextColored(SectionColor, label);
        ImGui.Separator();
    }

    /// <summary>
    /// A collapsible section header (icon + gold label). <paramref name="id"/> keeps the
    /// header's identity stable even if the visible label changes (e.g. a count). Returns
    /// whether it's open; wrap the body in <see cref="Body"/> so it reads as one section.
    /// </summary>
    public static bool CollapsingSection(FontAwesomeIcon icon, string label, string id, bool defaultOpen = true)
    {
        ImGui.Spacing();
        ImGui.AlignTextToFramePadding();
        Icon(icon, SectionColor);
        ImGui.SameLine(0, ImGui.GetStyle().ItemInnerSpacing.X * 1.5f);

        using var col = ImRaii.PushColor(ImGuiCol.Text, SectionColor);
        var flags = defaultOpen ? ImGuiTreeNodeFlags.DefaultOpen : ImGuiTreeNodeFlags.None;
        return ImGui.CollapsingHeader($"{label}###{id}", flags);
    }

    /// <summary>Indent a section's body so grouped controls read as one section.</summary>
    public static IDisposable Body() => ImRaii.PushIndent();

    // ── text hierarchy ─────────────────────────────────────────────────────────

    /// <summary>A field label — normal (bright) text, brighter than its help.</summary>
    public static void Label(string text) => ImGui.TextUnformatted(text);

    /// <summary>Secondary/help text — dimmed.</summary>
    public static void Help(string text) => ImGui.TextDisabled(text);

    /// <summary>A dimmed "(?)" that shows <paramref name="text"/> as a wrapped tooltip on hover.</summary>
    public static void HelpMarker(string text)
    {
        ImGui.TextDisabled("(?)");
        if (!ImGui.IsItemHovered())
            return;

        ImGui.BeginTooltip();
        ImGui.PushTextWrapPos(ImGui.GetFontSize() * 24f);
        ImGui.TextUnformatted(text);
        ImGui.PopTextWrapPos();
        ImGui.EndTooltip();
    }

    // ── buttons (three tiers) ────────────────────────────────────────────────────

    /// <summary>Primary call-to-action — the web accent (tan) fill with dark text on it.</summary>
    public static bool PrimaryButton(string label, Vector2 size = default)
    {
        using (ImRaii.PushColor(ImGuiCol.Button, Primary))
        using (ImRaii.PushColor(ImGuiCol.ButtonHovered, PrimaryHover))
        using (ImRaii.PushColor(ImGuiCol.ButtonActive, PrimaryActive))
        using (ImRaii.PushColor(ImGuiCol.Text, PrimaryText))
            return ImGui.Button(label, size);
    }

    /// <summary>Secondary action — the web olive accent-2 fill, bordered.</summary>
    public static bool Button(string label, Vector2 size = default)
    {
        using (ImRaii.PushStyle(ImGuiStyleVar.FrameBorderSize, 1f))
        using (ImRaii.PushColor(ImGuiCol.Button, Secondary))
        using (ImRaii.PushColor(ImGuiCol.ButtonHovered, SecondaryHover))
        using (ImRaii.PushColor(ImGuiCol.ButtonActive, SecondaryActive))
            return ImGui.Button(label, size);
    }

    /// <summary>Destructive / caution action — red + bordered.</summary>
    public static bool DangerButton(string label, Vector2 size = default)
    {
        using (ImRaii.PushStyle(ImGuiStyleVar.FrameBorderSize, 1f))
        using (ImRaii.PushColor(ImGuiCol.Button, Danger))
        using (ImRaii.PushColor(ImGuiCol.ButtonHovered, DangerHover))
        using (ImRaii.PushColor(ImGuiCol.ButtonActive, DangerActive))
            return ImGui.Button(label, size);
    }

    /// <summary>A compact secondary button (table rows, toolbars) — olive + bordered.</summary>
    public static bool SmallButton(string label)
    {
        using (ImRaii.PushStyle(ImGuiStyleVar.FrameBorderSize, 1f))
        using (ImRaii.PushColor(ImGuiCol.Button, Secondary))
        using (ImRaii.PushColor(ImGuiCol.ButtonHovered, SecondaryHover))
        using (ImRaii.PushColor(ImGuiCol.ButtonActive, SecondaryActive))
            return ImGui.SmallButton(label);
    }

    /// <summary>A compact icon button with a hover tooltip (neutral olive).</summary>
    public static bool IconButton(string id, FontAwesomeIcon icon, string tooltip = "")
    {
        var clicked = ImGuiComponents.IconButton(id, icon, Secondary, SecondaryActive, SecondaryHover);
        if (!string.IsNullOrEmpty(tooltip) && ImGui.IsItemHovered())
            ImGui.SetTooltip(tooltip);
        return clicked;
    }

    /// <summary>A compact icon button styled red for a destructive row action.</summary>
    public static bool DangerIconButton(string id, FontAwesomeIcon icon, string tooltip = "")
    {
        var clicked = ImGuiComponents.IconButton(id, icon, Danger, DangerActive, DangerHover);
        if (!string.IsNullOrEmpty(tooltip) && ImGui.IsItemHovered())
            ImGui.SetTooltip(tooltip);
        return clicked;
    }

    // ── badge ────────────────────────────────────────────────────────────────────

    /// <summary>
    /// A small pill (framed, tinted) in <paramref name="color"/> — for a channel tag or a
    /// run-status chip. Lays out as a single item, so <c>SameLine</c> works around it.
    /// </summary>
    public static void Badge(string text, Vector4 color)
    {
        var dl = ImGui.GetWindowDrawList();
        var pad = new Vector2(6f, 2f);
        var textSize = ImGui.CalcTextSize(text);
        var pos = ImGui.GetCursorScreenPos();
        var size = new Vector2(textSize.X + pad.X * 2f, textSize.Y + pad.Y * 2f);
        var fill = new Vector4(color.X, color.Y, color.Z, 0.18f);
        dl.AddRectFilled(pos, pos + size, ImGui.GetColorU32(fill), 4f);
        dl.AddRect(pos, pos + size, ImGui.GetColorU32(color), 4f);
        dl.AddText(new Vector2(pos.X + pad.X, pos.Y + pad.Y), ImGui.GetColorU32(color), text);
        ImGui.Dummy(size);
    }

    // ── card container ────────────────────────────────────────────────────────────

    /// <summary>
    /// Draws <paramref name="content"/> inside a rounded, faintly-filled, bordered card.
    ///
    /// This ImGui build has no <c>ImGuiChildFlags.AutoResizeY</c>, so a bordered child
    /// can't fit variable-height content. Instead the content is laid out inside a group
    /// on a foreground draw channel, then the fill + border are painted behind it on a
    /// background channel around the measured group rect — a version-independent card.
    /// Do not place a <c>BeginTable</c> inside a box: tables manage their own draw
    /// channels and would collide with the split. Ids inside are scoped by
    /// <paramref name="id"/>.
    /// </summary>
    public static void Box(string id, Action content)
    {
        var dl = ImGui.GetWindowDrawList();
        var pad = new Vector2(10f, 8f);
        var start = ImGui.GetCursorScreenPos();
        var width = Math.Max(1f, ImGui.GetContentRegionAvail().X);
        var innerWidth = Math.Max(1f, width - pad.X * 2f);

        dl.ChannelsSplit(2);
        dl.ChannelsSetCurrent(1); // content on top

        ImGui.SetCursorScreenPos(new Vector2(start.X + pad.X, start.Y + pad.Y));
        ImGui.BeginGroup();
        var localLeft = ImGui.GetCursorPosX();
        ImGui.PushTextWrapPos(localLeft + innerWidth);
        using (ImRaii.PushId(id))
            content();
        ImGui.PopTextWrapPos();
        ImGui.EndGroup();

        var bottom = ImGui.GetItemRectMax().Y + pad.Y;
        var max = new Vector2(start.X + width, bottom);

        dl.ChannelsSetCurrent(0); // fill + border behind
        dl.AddRectFilled(start, max, ImGui.GetColorU32(BoxBg), 6f);
        dl.AddRect(start, max, ImGui.GetColorU32(BoxBorder), 6f);
        dl.ChannelsMerge();

        // Advance the layout cursor past the card.
        ImGui.SetCursorScreenPos(new Vector2(start.X, max.Y));
        ImGui.Dummy(new Vector2(width, 0f));
        ImGui.Spacing();
    }
}
