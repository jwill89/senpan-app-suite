using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using System.Numerics;
using System.Threading.Tasks;
using Dalamud.Bindings.ImGui;
using Dalamud.Interface;
using SenpanCompanion.Api;
using SenpanCompanion.Core;
using SenpanCompanion.Services;

namespace SenpanCompanion.Windows;

/// <summary>
/// Raffle operator panel: pick a raffle, add entrants (with nearby-player quick-fill),
/// record payments, and draw a winner (pick -> confirm, or pick again). Matches the
/// website's admin raffle entry flow; raffle creation stays on the website by design.
/// The winner being confirmed is the server's, so a pick made on any console shows
/// here, and confirming or redrawing names it. A raffles change from anywhere marks the
/// page stale; Refresh re-pulls the list and the open raffle on demand.
/// </summary>
internal sealed class RaffleTab : TabBase
{
    private readonly SenpanApi api;
    private readonly NearbyPlayers nearby;

    private List<Raffle> raffles = new();
    private long selectedRaffleId;
    private RaffleDetail? detail;
    private List<RaffleEntry> entries = new();

    private string charName = string.Empty;
    private string world = string.Empty;
    private int numEntries = 1;
    private bool markPaidOnAdd = true;

    // Payment popup form state. One set is enough: only one popup is open at a time,
    // and OpenPaymentPopup reseeds both fields for the entry being settled.
    private int payTickets = 1;
    private int payWaived;

    public RaffleTab(SenpanApi api, NearbyPlayers nearby)
    {
        this.api = api;
        this.nearby = nearby;
    }

    /// <summary>Reloads the raffle list and, if one is open, its detail and entries.</summary>
    protected override async Task LoadAsync()
    {
        var selected = this.selectedRaffleId;
        var rafflesRes = await this.api.ListRafflesAsync();
        (RaffleDetail Detail, List<RaffleEntry> Entries)? opened = selected != 0 ? await this.ReadRaffleAsync(selected) : null;
        await Apply(() =>
        {
            this.raffles = rafflesRes.Raffles.ToList();
            // Only if the selection has not moved on while this was in flight.
            if (opened is { } read && this.selectedRaffleId == selected)
                (this.detail, this.entries) = read;
        });
    }

    private async Task<(RaffleDetail Detail, List<RaffleEntry> Entries)> ReadRaffleAsync(long id)
    {
        var detail = await this.api.GetRaffleAsync(id);
        var entries = await this.api.ListRaffleEntriesAsync(id);
        return (detail, entries.Entries.ToList());
    }

    /// <summary>Reads the raffle again after a change, keeping it only if it is still the one shown.</summary>
    private async Task ReloadRaffleAsync(long id)
    {
        var (detail, entries) = await this.ReadRaffleAsync(id);
        await Apply(() =>
        {
            if (this.selectedRaffleId != id)
                return;
            this.detail = detail;
            this.entries = entries;
        });
    }

    /// <summary>The winner picked and not yet confirmed, from any console; null for none.</summary>
    private RaffleEntry? PendingWinner =>
        this.detail is { Raffle: { Status: RaffleStatus.Open, WinnerEntryId: { } id } }
            ? this.entries.FirstOrDefault(e => e.Id == id)
            : null;

    public void Draw()
    {
        this.DrawStatusLine();

        if (Ui.Button("Refresh##raffles"))
            this.Run(this.LoadAsync);
        ImGui.SameLine();
        this.DrawRafflePicker();

        if (this.detail == null)
        {
            ImGui.TextDisabled("Select a raffle.");
            return;
        }

        ImGui.Separator();
        DrawRaffleHeader(this.detail);

        Ui.Section(FontAwesomeIcon.UserPlus, "Add entrant");
        this.DrawAddEntry();

        Ui.Section(FontAwesomeIcon.Users, "Entrants");
        this.DrawEntries(this.detail.Raffle);

        Ui.Section(FontAwesomeIcon.Trophy, "Draw a winner");
        this.DrawWinnerControls();
    }

    private void DrawRafflePicker()
    {
        var current = this.raffles.FirstOrDefault(r => r.Id == this.selectedRaffleId);
        var preview = current != null ? $"{current.Title} ({StatusWord(current.Status)})" : "Select raffle...";

        ImGui.SetNextItemWidth(280);
        if (!ImGui.BeginCombo("##rafflepick", preview))
            return;
        foreach (var raffle in this.raffles)
        {
            var selected = raffle.Id == this.selectedRaffleId;
            if (ImGui.Selectable($"{raffle.Title} ({StatusWord(raffle.Status)})##r{raffle.Id}", selected))
                this.LoadRaffle(raffle.Id);
        }
        ImGui.EndCombo();
    }

    private void LoadRaffle(long id)
    {
        this.selectedRaffleId = id;
        // Clear the previous raffle's detail immediately, and only write back a fetch
        // that still matches the selection, so "Add entrant" and "Pick a winner" never
        // act on a raffle that isn't on screen.
        this.detail = null;
        this.entries = new List<RaffleEntry>();
        this.Run(() => this.ReloadRaffleAsync(id));
    }

    private static string StatusWord(RaffleStatus status) => status == RaffleStatus.Open ? "open" : "closed";

    private static void DrawRaffleHeader(RaffleDetail d)
    {
        ImGui.Text($"{d.Raffle.Title}");
        ImGui.SameLine();
        ImGui.TextDisabled($"- {StatusWord(d.Raffle.Status)}, {d.TotalEntries} entr{(d.TotalEntries == 1 ? "y" : "ies")}");

        var costs = DescribeCost(d.Raffle);
        if (costs.Length > 0)
            ImGui.TextDisabled($"{costs}  *  Max per person: {d.Raffle.MaxEntries}");
        else if (!d.Raffle.AcceptsSignups())
            ImGui.TextDisabled($"Details only - players sign up elsewhere  *  Max per person: {d.Raffle.MaxEntries}");
    }

    /// <summary>
    /// The raffle's pricing as one line: a flat cost per entry, or the whole
    /// ladder when each entry has its own price. Empty when nothing is charged.
    /// </summary>
    private static string DescribeCost(Raffle raffle)
    {
        if (raffle.EntryMode == RaffleEntryMode.Custom)
        {
            return raffle.TierCosts.Count == 0
                ? string.Empty
                : "Cost per entry: " + string.Join(" / ", raffle.TierCosts.Select(Gil));
        }

        return raffle.AcceptsSignups() && raffle.CostPerEntry > 0
            ? $"Cost per entry: {Gil(raffle.CostPerEntry)}"
            : string.Empty;
    }

    private static string Gil(long amount) => amount.ToString("N0", CultureInfo.InvariantCulture);

    private void DrawAddEntry()
    {
        if (this.detail?.Raffle.Status != RaffleStatus.Open)
        {
            UiText.WrappedDisabled("This raffle is closed - entries can't be added.");
            return;
        }

        // A details-only raffle is published for reference and entered somewhere else
        // entirely, so nobody is signed up through the app. Say why, rather than
        // offering a form that would quietly create an entrant this raffle never
        // meant to collect. (Entrants for one are recorded on the website.)
        if (!this.detail.Raffle.AcceptsSignups())
        {
            UiText.WrappedDisabled(
                "This raffle is details only - players enter it outside the app, so there is " +
                "no sign-up to take here.");
            return;
        }

        ImGui.SetNextItemWidth(160);
        ImGui.InputText("Name##entry", ref this.charName, 64);
        ImGui.SameLine();
        ImGui.SetNextItemWidth(140);
        ImGui.InputText("World##entry", ref this.world, 32);
        ImGui.SameLine();
        this.DrawNearbyPicker();

        ImGui.SetNextItemWidth(120);
        if (ImGui.InputInt("Tickets", ref this.numEntries))
            this.numEntries = Math.Max(1, this.numEntries);
        ImGui.SameLine();
        ImGui.Checkbox("Paid", ref this.markPaidOnAdd);
        ImGui.SameLine();

        var canAdd = !string.IsNullOrWhiteSpace(this.charName) && !string.IsNullOrWhiteSpace(this.world);
        if (!canAdd)
            ImGui.BeginDisabled();
        if (Ui.PrimaryButton("Add entrant"))
        {
            var id = this.selectedRaffleId;
            var entry = new RaffleEntryInput
            {
                CharacterName = this.charName.Trim(),
                World = this.world.Trim(),
                NumEntries = Math.Max(1, this.numEntries),
                Paid = this.markPaidOnAdd,
            };
            this.Run(async () =>
            {
                await this.api.AddRaffleEntryAsync(id, entry);
                await this.ReloadRaffleAsync(id);
                await Apply(() =>
                {
                    this.charName = string.Empty;
                    this.world = string.Empty;
                    this.numEntries = 1;
                });
            });
        }
        if (!canAdd)
            ImGui.EndDisabled();
    }

    private void DrawEntries(Raffle raffle)
    {
        if (this.entries.Count == 0)
        {
            ImGui.TextDisabled("No entries yet.");
            return;
        }

        if (!ImGui.BeginTable("entries", 5,
                ImGuiTableFlags.Borders | ImGuiTableFlags.RowBg | ImGuiTableFlags.ScrollY,
                new Vector2(0, 200)))
        {
            return;
        }

        ImGui.TableSetupColumn("Name");
        ImGui.TableSetupColumn("World");
        ImGui.TableSetupColumn("Tickets", ImGuiTableColumnFlags.WidthFixed, 70);
        ImGui.TableSetupColumn("Paid", ImGuiTableColumnFlags.WidthFixed, 90);
        ImGui.TableSetupColumn("##actions", ImGuiTableColumnFlags.WidthFixed, 70);
        ImGui.TableHeadersRow();

        foreach (var entry in this.entries)
        {
            ImGui.TableNextRow();
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(entry.CharacterName);
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(entry.World);
            ImGui.TableNextColumn();
            // Entries merge per character+world, so an entrant who settled up and
            // then bought more tickets is only PARTLY paid - show both numbers
            // rather than a bare total that hides the outstanding ones.
            if (entry.PaymentState == RaffleEntryPaymentState.Partial)
            {
                ImGui.TextUnformatted($"{entry.PaidEntries} / {entry.NumEntries}");
                Ui.ItemTooltip($"{entry.PaidEntries} of {entry.NumEntries} tickets settled");
            }
            else
            {
                ImGui.TextUnformatted(entry.NumEntries.ToString(CultureInfo.InvariantCulture));
            }

            ImGui.TableNextColumn();
            this.DrawPaymentCell(raffle, entry);

            ImGui.TableNextColumn();
            if (Ui.DangerIconButton($"e{entry.Id}", FontAwesomeIcon.Trash, "Delete entrant"))
            {
                var id = this.selectedRaffleId;
                var entryId = entry.Id;
                this.Run(async () =>
                {
                    await this.api.DeleteRaffleEntryAsync(id, entryId);
                    await this.ReloadRaffleAsync(id);
                });
            }
        }

        ImGui.EndTable();
    }

    // -- Payment -------------------------------------------------------------

    /// <summary>
    /// The Paid column: a button showing how far the entry has settled, opening a
    /// popup that records a payment. The popup is where part payments and waived gil
    /// live - both need a number typed, which a checkbox has nowhere to put.
    /// </summary>
    private void DrawPaymentCell(Raffle raffle, RaffleEntry entry)
    {
        var (label, color) = entry.PaymentState switch
        {
            RaffleEntryPaymentState.Paid => ("Paid", Ui.SuccessColor),
            RaffleEntryPaymentState.Partial => ("Partial", Ui.WarnColor),
            _ => ("Unpaid", Ui.InfoColor),
        };

        ImGui.PushStyleColor(ImGuiCol.Text, color);
        var open = ImGui.SmallButton($"{label}##pay{entry.Id}");
        ImGui.PopStyleColor();
        Ui.ItemTooltip("Record a payment");
        if (open)
            this.OpenPaymentPopup(entry);

        this.DrawPaymentPopup(raffle, entry);
    }

    /// <summary>
    /// Seeds the popup for one entry and opens it. The ticket box starts at
    /// everything outstanding (the common case - somebody paying up in full) and the
    /// waiver starts at zero, never at what was waived before: the server ADDS what
    /// is sent, so re-sending the running total would double it.
    /// </summary>
    private void OpenPaymentPopup(RaffleEntry entry)
    {
        this.payTickets = (int)Math.Max(1, entry.NumEntries);
        this.payWaived = 0;
        ImGui.OpenPopup(PaymentPopupId(entry));
    }

    private static string PaymentPopupId(RaffleEntry entry) => $"##paypopup{entry.Id}";

    private void DrawPaymentPopup(Raffle raffle, RaffleEntry entry)
    {
        if (!ImGui.BeginPopup(PaymentPopupId(entry)))
            return;

        ImGui.TextUnformatted($"{entry.CharacterName} @ {entry.World}");
        Ui.Help($"{entry.NumEntries} ticket{(entry.NumEntries == 1 ? string.Empty : "s")}"
                + (entry.PaidEntries > 0 ? $"  -  {entry.PaidEntries} already settled" : string.Empty));

        var charges = raffle.Charges();
        if (charges)
        {
            var outstanding = raffle.EntryCost(entry.NumEntries) - raffle.EntryCost(entry.PaidEntries);
            Ui.Help($"Outstanding: {Gil(Math.Max(0, outstanding))}"
                    + (entry.AmountWaived > 0 ? $"  -  {Gil(entry.AmountWaived)} waived so far" : string.Empty));
        }

        ImGui.Separator();

        // Part payments: how many of the tickets this settlement covers. Only worth
        // a control when there is more than one left to pay for.
        if (entry.NumEntries > 1)
        {
            ImGui.SetNextItemWidth(120);
            if (ImGui.InputInt("Tickets paid for", ref this.payTickets))
                this.payTickets = (int)Math.Clamp(this.payTickets, 1, entry.NumEntries);
            if (this.payTickets < entry.NumEntries)
                Ui.Help($"Leaves {entry.NumEntries - this.payTickets} outstanding.");
        }

        if (charges)
        {
            ImGui.SetNextItemWidth(140);
            if (ImGui.InputInt("Amount waived", ref this.payWaived, 1000, 10000))
                this.payWaived = Math.Max(0, this.payWaived);
            Ui.Help("Forgiven on this payment only - added to anything waived before.");
        }

        ImGui.Separator();

        if (Ui.PrimaryButton("Mark paid"))
        {
            // Every ticket unless fewer were typed: a part payment names its count.
            long? tickets = this.payTickets < entry.NumEntries ? this.payTickets : null;
            this.Settle(entry.Id, new RaffleSettlement { PaidEntries = tickets, Waived = this.payWaived });
            ImGui.CloseCurrentPopup();
        }
        if (entry.PaidEntries > 0)
        {
            ImGui.SameLine();
            if (Ui.DangerButton("Clear payment"))
            {
                // Clearing resets the whole row - settled tickets AND waived gil -
                // which is also the only way to walk a settlement back.
                this.Settle(entry.Id, null);
                ImGui.CloseCurrentPopup();
            }
        }
        ImGui.SameLine();
        if (Ui.Button("Cancel"))
            ImGui.CloseCurrentPopup();

        ImGui.EndPopup();
    }

    /// <summary>Records one settlement, or clears the entry's when <paramref name="settlement"/>
    /// is null, then re-reads the raffle so the table reflects it.</summary>
    private void Settle(long entryId, RaffleSettlement? settlement)
    {
        var id = this.selectedRaffleId;
        this.Run(async () =>
        {
            if (settlement != null)
                await this.api.SettleRaffleEntryAsync(id, entryId, settlement);
            else
                await this.api.UnsettleRaffleEntryAsync(id, entryId);
            await this.ReloadRaffleAsync(id);
        });
    }

    private void DrawWinnerControls()
    {
        if (this.detail?.Raffle.Status == RaffleStatus.Closed)
        {
            ImGui.TextDisabled(this.detail.WinnerEntry is { } won
                ? $"Closed. Winner: {won.CharacterName} @ {won.World}"
                : "Closed without a winner.");
            return;
        }

        if (this.PendingWinner is { } pending)
        {
            UiText.WrappedColored(new Vector4(0.3f, 0.9f, 0.4f, 1f),
                $"Drawn winner: {pending.CharacterName} @ {pending.World}");

            var id = this.selectedRaffleId;
            if (Ui.PrimaryButton("Confirm winner"))
            {
                var confirm = new RaffleWinnerConfirm { WinnerEntryId = pending.Id };
                this.Run(async () =>
                {
                    await this.api.VerifyRaffleWinnerAsync(id, confirm);
                    await this.ReloadRaffleAsync(id);
                });
            }
            ImGui.SameLine();
            if (Ui.Button("Draw another"))
            {
                var choice = new RaffleWinnerChoice { Replaces = pending.Id };
                this.Run(async () =>
                {
                    await this.api.PickAnotherRaffleWinnerAsync(id, choice);
                    await this.ReloadRaffleAsync(id);
                });
            }
            return;
        }

        if (Ui.PrimaryButton("Pick a winner"))
        {
            var id = this.selectedRaffleId;
            this.Run(async () =>
            {
                await this.api.PickRaffleWinnerAsync(id);
                await this.ReloadRaffleAsync(id);
            });
        }
        ImGui.SameLine();
        // A details-only raffle collects nothing through the app, so every entry is
        // eligible; anything that charges draws on tickets that were settled.
        ImGui.TextDisabled(this.detail?.Raffle.AcceptsSignups() == false
            ? "Draws from every entrant."
            : "Draws on paid tickets only.");
    }

    private void DrawNearbyPicker()
    {
        if (!ImGui.BeginCombo("##rafflenearby", "Nearby...", ImGuiComboFlags.NoArrowButton))
            return;
        foreach (var np in this.nearby.Snapshot())
        {
            if (ImGui.Selectable($"{np.Name} ({np.World})"))
            {
                this.charName = np.Name;
                this.world = np.World;
            }
        }
        ImGui.EndCombo();
    }
}
