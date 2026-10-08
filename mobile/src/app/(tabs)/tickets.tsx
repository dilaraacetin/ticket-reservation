// What the caller has: seats confirmed, and seats still only held. The API
// answers with both, so this screen shows both and says which is which.

import { router, useFocusEffect } from 'expo-router';
import { useCallback, useState } from 'react';
import { FlatList, Image, RefreshControl, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Caption, Empty, Label, Muted, Tag, Title } from '@/components/ui';
import { ApiError, api, assetUrl, authHeader } from '@/lib/api';
import { useSession } from '@/lib/session';
import { color, radius, space } from '@/lib/theme';
import type { Ticket, Tickets } from '@/lib/types';

export default function MyTickets() {
  const { account, ready } = useSession();
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    if (!account) {
      setTickets([]);
      setLoading(false);

      return;
    }

    try {
      const body = await api<Tickets>('GET', '/tickets');
      setTickets(body.reservations);
    } catch (err) {
      if (!(err instanceof ApiError)) setTickets([]);
    } finally {
      setLoading(false);
    }
  }, [account]);

  useFocusEffect(
    useCallback(() => {
      load();
    }, [load]),
  );

  if (!ready) return <SafeAreaView style={st.safe} edges={['top']} />;

  if (!account) {
    return (
      <SafeAreaView style={st.safe} edges={['top']}>
        <View style={st.pad}>
          <Title>My tickets</Title>
          <Empty
            icon="ticket-outline"
            title="Nothing here yet"
            text="Sign in to see the seats you are holding and the tickets you own."
            action={{ label: 'Sign in', onPress: () => router.push('/sign-in') }}
          />
        </View>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={st.safe} edges={['top']}>
      <FlatList
        data={tickets}
        keyExtractor={(t) => `${t.eventId}:${t.seatId}`}
        contentContainerStyle={st.pad}
        refreshControl={<RefreshControl refreshing={loading} onRefresh={load} tintColor={color.textMuted} />}
        ListHeaderComponent={<Title style={st.head}>My tickets</Title>}
        ListEmptyComponent={
          loading ? null : (
            <Empty
              icon="ticket-outline"
              title="No tickets yet"
              text="Hold a seat and confirm it, and it shows up here ready to scan."
              action={{ label: 'Browse events', onPress: () => router.push('/') }}
            />
          )
        }
        renderItem={({ item }) => <TicketCard ticket={item} />}
      />
    </SafeAreaView>
  );
}

function TicketCard({ ticket }: { ticket: Ticket }) {
  const when = new Date(ticket.startsAt);
  const held = ticket.status === 'held';

  return (
    <View style={st.ticket}>
      <View style={st.ticketHead}>
        <View style={st.fill}>
          <Label>
            {when.toLocaleString(undefined, { weekday: 'short', day: 'numeric', month: 'short' })}
          </Label>
          <Text style={st.eventName}>{ticket.eventName}</Text>
          <Muted style={st.where}>
            {ticket.venue} · {when.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })}
          </Muted>
        </View>

        <View style={st.seatNo}>
          <Caption>SEAT</Caption>
          <Text style={st.seatDigits}>{ticket.seatId}</Text>
        </View>
      </View>

      <View style={st.tear} />

      <View style={st.stub}>
        {held ? (
          <View style={st.fill}>
            <Tag text="Held, not confirmed" tone="danger" />
            <Muted style={st.heldNote}>
              This seat is still only held. Confirm it before the hold runs out, or it goes back on sale.
            </Muted>
          </View>
        ) : (
          <>
            <Image
              source={{ uri: assetUrl(`/tickets/${ticket.ticketCode}/qr.png`), headers: authHeader() }}
              style={st.qr}
            />
            <View style={st.fill}>
              <Caption>TICKET CODE</Caption>
              <Text style={st.code}>{ticket.ticketCode}</Text>
              <Muted style={st.scan}>Show this at the door.</Muted>
            </View>
          </>
        )}
      </View>
    </View>
  );
}

const st = StyleSheet.create({
  safe: { flex: 1, backgroundColor: color.bg },
  pad: { padding: space.gutter, paddingBottom: 32 },
  head: { marginBottom: 18 },
  fill: { flex: 1 },

  ticket: {
    backgroundColor: color.surface,
    borderWidth: 1,
    borderColor: color.line,
    borderRadius: radius.lg,
    overflow: 'hidden',
    marginBottom: 14,
  },
  ticketHead: { flexDirection: 'row', gap: 14, padding: 18 },
  eventName: { fontSize: 22, lineHeight: 27, fontWeight: '700', color: color.text, marginTop: 6 },
  where: { fontSize: 14 },

  seatNo: { alignItems: 'flex-end' },
  seatDigits: { fontSize: 30, fontWeight: '700', color: color.primary },

  tear: { marginHorizontal: 18, borderTopWidth: 1, borderStyle: 'dashed', borderColor: color.lineStrong },

  stub: { flexDirection: 'row', gap: 16, padding: 18, alignItems: 'center' },
  qr: { width: 108, height: 108, borderRadius: 10, backgroundColor: '#FFFFFF' },
  code: { fontSize: 19, fontWeight: '600', letterSpacing: 1.1, color: color.text, marginTop: 2 },
  scan: { fontSize: 13, marginTop: 4 },

  heldNote: { fontSize: 14, marginTop: 8 },
});
