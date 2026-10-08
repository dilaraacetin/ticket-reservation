// The seat map, and the countdown that owns the one loud colour.
//
// Four states, and each differs in shape and symbol as well as colour, so the
// map still reads with the colour taken away.

import { Ionicons } from '@expo/vector-icons';
import { router, useLocalSearchParams } from 'expo-router';
import { useCallback, useEffect, useRef, useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Banner, Button, Caption, Muted } from '@/components/ui';
import { ApiError, api, newIdempotencyKey } from '@/lib/api';
import { useSession } from '@/lib/session';
import { blend, color, radius, space } from '@/lib/theme';
import type { EventDetail, Hold, Seat, SeatMap, Tickets } from '@/lib/types';

// Where the aisle goes: after the fifth seat, which is what makes a seat map
// read as a room rather than a spreadsheet.
const AISLE_AFTER = 5;

export default function Seats() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { account } = useSession();

  const [event, setEvent] = useState<EventDetail | null>(null);
  const [seats, setSeats] = useState<Seat[]>([]);
  const [hold, setHold] = useState<Hold | null>(null);
  const [left, setLeft] = useState(0);
  const [notice, setNotice] = useState<{ kind: 'error' | 'success'; title: string; text?: string } | null>(null);
  const [working, setWorking] = useState(false);
  const [loading, setLoading] = useState(true);

  const total = useRef(0);

  const loadSeats = useCallback(async () => {
    const map = await api<SeatMap>('GET', `/events/${encodeURIComponent(id)}/seats`);
    setSeats(map.seats);
  }, [id]);

  // A hold survives the app being closed, so it is recovered rather than
  // assumed gone: /tickets answers with held seats as well as confirmed ones.
  const recoverHold = useCallback(async () => {
    if (!account) return;

    try {
      const body = await api<Tickets>('GET', '/tickets');
      const mine = body.reservations.find((t) => t.eventId === id && t.status === 'held' && t.holdId);

      if (mine?.holdId && mine.expiresAt) {
        setHold({
          holdId: mine.holdId,
          eventId: id,
          seatId: mine.seatId,
          userId: '',
          expiresAt: mine.expiresAt,
          expiresInSeconds: mine.expiresInSeconds ?? 0,
        });
        total.current = mine.expiresInSeconds ?? 0;
      }
    } catch {
      // Not being able to recover a hold is not a reason to refuse the screen.
    }
  }, [account, id]);

  useEffect(() => {
    (async () => {
      try {
        const [detail] = await Promise.all([
          api<EventDetail>('GET', `/events/${encodeURIComponent(id)}`),
          loadSeats(),
        ]);
        setEvent(detail);
        await recoverHold();
      } catch {
        setNotice({ kind: 'error', title: 'Could not load the seat map' });
      } finally {
        setLoading(false);
      }
    })();
  }, [id, loadSeats, recoverHold]);

  // The countdown. Driven by the expiry rather than by counting ticks, so a
  // phone that slept does not come back believing it has time it does not.
  useEffect(() => {
    if (!hold) return;

    const tick = () => {
      const seconds = Math.max(0, Math.round((new Date(hold.expiresAt).getTime() - Date.now()) / 1000));
      setLeft(seconds);

      if (seconds === 0) {
        setHold(null);
        setNotice({ kind: 'error', title: 'The hold ran out', text: 'The seat went back on sale.' });
        loadSeats();
      }
    };

    tick();
    const timer = setInterval(tick, 1000);

    return () => clearInterval(timer);
  }, [hold, loadSeats]);

  const take = async (seat: Seat) => {
    if (!account) {
      router.push('/sign-in');

      return;
    }

    setNotice(null);
    setWorking(true);

    try {
      const taken = await api<Hold>(
        'POST',
        `/events/${encodeURIComponent(id)}/seats/${encodeURIComponent(seat.id)}/hold`,
        { idempotencyKey: newIdempotencyKey() },
      );

      setHold(taken);
      total.current = taken.expiresInSeconds;
      await loadSeats();
    } catch (err) {
      setNotice({
        kind: 'error',
        title: 'That seat got away',
        text: err instanceof ApiError ? err.message : 'Something went wrong.',
      });
      await loadSeats();
    } finally {
      setWorking(false);
    }
  };

  const confirm = async () => {
    if (!hold) return;

    setWorking(true);

    try {
      await api('POST', `/holds/${encodeURIComponent(hold.holdId)}/confirm`, {
        idempotencyKey: newIdempotencyKey(),
      });

      setHold(null);
      setNotice({ kind: 'success', title: `Seat ${hold.seatId} is yours`, text: "It's in My tickets, ready to scan." });
      await loadSeats();
    } catch (err) {
      setNotice({
        kind: 'error',
        title: 'Could not confirm',
        text: err instanceof ApiError ? err.message : 'Something went wrong.',
      });
      await loadSeats();
    } finally {
      setWorking(false);
    }
  };

  const release = async () => {
    if (!hold) return;

    setWorking(true);

    try {
      await api('DELETE', `/holds/${encodeURIComponent(hold.holdId)}`);
      setHold(null);
      await loadSeats();
    } finally {
      setWorking(false);
    }
  };

  if (loading) {
    return (
      <View style={[st.safe, st.centre]}>
        <ActivityIndicator color={color.primary} />
      </View>
    );
  }

  const rows = new Map<string, Seat[]>();
  for (const seat of seats) {
    if (!rows.has(seat.row)) rows.set(seat.row, []);
    rows.get(seat.row)!.push(seat);
  }

  const open = seats.filter((s) => s.status === 'available').length;

  return (
    <SafeAreaView style={st.safe} edges={['top', 'bottom']}>
      <View style={st.head}>
        <Pressable onPress={() => router.back()} hitSlop={12} accessibilityLabel="Back to the event">
          <Ionicons name="chevron-back" size={26} color={color.text} />
        </Pressable>
        <View style={st.fill}>
          <Text style={st.headTitle} numberOfLines={1}>
            {event?.name ?? 'Seats'}
          </Text>
          <Caption>{event ? `${event.venue} · ${event.city}` : ''}</Caption>
        </View>
        <Pressable onPress={loadSeats} hitSlop={12} accessibilityLabel="Refresh the seat map">
          <Ionicons name="refresh" size={22} color={color.textMuted} />
        </Pressable>
      </View>

      <ScrollView contentContainerStyle={st.scroll}>
        {notice ? <Banner kind={notice.kind} title={notice.title} text={notice.text} /> : null}

        <View style={st.counts}>
          <Text style={[st.countsN, open === 0 && { color: color.danger }]}>{open}</Text>
          <Muted> open of {seats.length}</Muted>
        </View>

        <View style={st.stage}>
          <Text style={st.stageText}>STAGE</Text>
        </View>

        <ScrollView horizontal showsHorizontalScrollIndicator={false}>
          <View style={st.grid}>
            {[...rows.entries()].map(([label, rowSeats]) => (
              <View key={label} style={st.row}>
                <Text style={st.rowLabel}>{label}</Text>
                {rowSeats.map((seat, index) => (
                  <View key={seat.id} style={st.seatWrap}>
                    {index === AISLE_AFTER ? <View style={st.aisle} /> : null}
                    <SeatButton
                      seat={seat}
                      mine={hold?.seatId === seat.id}
                      locked={Boolean(hold) || working}
                      onPress={() => take(seat)}
                    />
                  </View>
                ))}
              </View>
            ))}
          </View>
        </ScrollView>

        <View style={st.legend}>
          <Key fill={color.seatOpen} text="Open" />
          <Key fill={color.seatHeldFill} border={color.seatHeldLine} text="Held" />
          <Key fill={color.seatSold} text="Sold" />
          <Key fill={color.seatMine} text="Yours" />
        </View>
      </ScrollView>

      {hold ? (
        <HoldBar
          seatId={hold.seatId}
          left={left}
          total={total.current || 1}
          busy={working}
          onConfirm={confirm}
          onRelease={release}
        />
      ) : null}
    </SafeAreaView>
  );
}

function SeatButton({ seat, mine, locked, onPress }: { seat: Seat; mine: boolean; locked: boolean; onPress: () => void }) {
  const state = mine ? 'mine' : seat.status;
  const off = state !== 'available' || locked;

  return (
    <Pressable
      disabled={off}
      onPress={onPress}
      accessibilityLabel={`Seat ${seat.id}, ${mine ? 'yours' : seat.status}`}
      style={({ pressed }) => [
        st.seat,
        state === 'available' && st.seatOpen,
        state === 'held' && st.seatHeld,
        state === 'reserved' && st.seatSold,
        state === 'mine' && st.seatMine,
        pressed && !off && { transform: [{ scale: 1.08 }] },
      ]}>
      {state === 'held' ? <Ionicons name="time-outline" size={13} color={color.seatHeldLine} /> : null}
      {state === 'reserved' ? <Ionicons name="close" size={13} color={color.textMuted} /> : null}
      {state === 'mine' ? <Ionicons name="checkmark" size={14} color={color.onPrimary} /> : null}
    </Pressable>
  );
}

function Key({ fill, border, text }: { fill: string; border?: string; text: string }) {
  return (
    <View style={st.key}>
      <View
        style={[
          st.keySwatch,
          { backgroundColor: fill },
          border ? { borderWidth: 1, borderStyle: 'dashed', borderColor: border } : null,
        ]}
      />
      <Caption>{text}</Caption>
    </View>
  );
}

function HoldBar({
  seatId,
  left,
  total,
  busy,
  onConfirm,
  onRelease,
}: {
  seatId: string;
  left: number;
  total: number;
  busy: boolean;
  onConfirm: () => void;
  onRelease: () => void;
}) {
  const urgent = left <= 60;
  const digits = `${Math.floor(left / 60)}:${String(left % 60).padStart(2, '0')}`;

  return (
    <View style={st.holdbar}>
      <View style={st.holdTop}>
        <View style={st.fill}>
          <Caption>HOLDING</Caption>
          <Text style={st.holdSeat}>Seat {seatId}</Text>
        </View>
        <View>
          <Text style={[st.holdClock, urgent && { color: color.danger }]}>{digits}</Text>
          <Caption>left</Caption>
        </View>
      </View>

      <View style={st.holdMeter}>
        <View
          style={[
            st.holdMeterFill,
            { width: `${Math.max(0, Math.min(100, (left / total) * 100))}%` },
            urgent && { backgroundColor: color.danger },
          ]}
        />
      </View>

      <View style={st.holdActions}>
        <Button title="Give up" variant="danger" onPress={onRelease} disabled={busy} style={st.holdGiveUp} />
        <Button title="Confirm" onPress={onConfirm} busy={busy} style={st.fill} />
      </View>
    </View>
  );
}

const SEAT = 30;

const st = StyleSheet.create({
  safe: { flex: 1, backgroundColor: color.bg },
  centre: { alignItems: 'center', justifyContent: 'center' },
  fill: { flex: 1 },

  head: { flexDirection: 'row', alignItems: 'center', gap: 12, padding: space.gutter },
  headTitle: { fontSize: 22, fontWeight: '700', color: color.text },

  scroll: { paddingHorizontal: space.gutter, paddingBottom: 220 },

  counts: { flexDirection: 'row', alignItems: 'baseline', justifyContent: 'center', paddingVertical: 10 },
  countsN: { fontSize: 20, fontWeight: '700', color: color.text },

  stage: {
    alignSelf: 'center',
    width: '85%',
    height: 44,
    borderBottomWidth: 2,
    borderBottomColor: color.primary,
    borderBottomLeftRadius: 120,
    borderBottomRightRadius: 120,
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: 22,
  },
  stageText: { fontSize: 13, fontWeight: '700', letterSpacing: 4, color: color.primary },

  grid: { gap: 6, paddingBottom: 4 },
  row: { flexDirection: 'row', alignItems: 'center', gap: 6 },
  rowLabel: { width: 18, textAlign: 'center', fontSize: 13, fontWeight: '600', color: color.textMuted },
  seatWrap: { flexDirection: 'row', alignItems: 'center' },
  aisle: { width: 14 },

  seat: {
    width: SEAT,
    height: SEAT,
    marginRight: 6,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: 'transparent',
    alignItems: 'center',
    justifyContent: 'center',
  },
  seatOpen: { backgroundColor: color.seatOpen },
  seatHeld: { backgroundColor: color.seatHeldFill, borderStyle: 'dashed', borderColor: color.seatHeldLine },
  seatSold: { backgroundColor: color.seatSold, borderColor: color.seatSold },
  seatMine: { backgroundColor: color.seatMine },

  legend: { flexDirection: 'row', flexWrap: 'wrap', justifyContent: 'center', gap: 18, paddingTop: 20 },
  key: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  keySwatch: { width: 18, height: 18, borderRadius: 5 },

  holdbar: {
    position: 'absolute',
    left: space.gutter,
    right: space.gutter,
    bottom: 20,
    backgroundColor: color.surfaceRaised,
    borderWidth: 1,
    borderColor: color.lineStrong,
    borderRadius: radius.lg,
    padding: 16,
    gap: 14,
  },
  holdTop: { flexDirection: 'row', alignItems: 'center', gap: 14 },
  holdSeat: { fontSize: 19, fontWeight: '700', color: color.text },
  holdClock: { fontSize: 36, fontWeight: '700', color: color.text, textAlign: 'right' },

  holdMeter: { height: 5, borderRadius: 3, backgroundColor: color.line, overflow: 'hidden' },
  holdMeterFill: { height: '100%', backgroundColor: color.primary },

  holdActions: { flexDirection: 'row', gap: 10 },
  holdGiveUp: { flex: 0, paddingHorizontal: 18, borderColor: blend.dangerSoft },
});
