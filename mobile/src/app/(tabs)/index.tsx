// The catalogue. A poster, a date chip over it, and the words underneath, with
// two filters built from what is actually on sale.

import { Ionicons } from '@expo/vector-icons';
import { router, useFocusEffect } from 'expo-router';
import { useCallback, useMemo, useState } from 'react';
import { FlatList, Pressable, RefreshControl, ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Banner, Caption, Empty, Muted, Poster, Tag, Title } from '@/components/ui';
import { api } from '@/lib/api';
import { color, radius, space } from '@/lib/theme';
import { CATEGORY_NAMES, CATEGORY_ORDER, type Category, type Event } from '@/lib/types';

export default function Events() {
  const [events, setEvents] = useState<Event[]>([]);
  const [failed, setFailed] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [category, setCategory] = useState<Category | ''>('');
  const [city, setCity] = useState('');

  const load = useCallback(async () => {
    try {
      setEvents(await api<Event[]>('GET', '/events'));
      setFailed(null);
    } catch {
      setFailed('Could not reach the server. Check that it is running and on the same network.');
    } finally {
      setLoading(false);
    }
  }, []);

  useFocusEffect(
    useCallback(() => {
      load();
    }, [load]),
  );

  // Both filters come from what is on sale, so neither can offer something that
  // matches nothing.
  const kinds = useMemo(
    () => CATEGORY_ORDER.filter((k) => events.some((e) => e.category === k)),
    [events],
  );
  const cities = useMemo(
    () => [...new Set(events.map((e) => e.city).filter(Boolean))].sort(),
    [events],
  );

  const shown = useMemo(
    () => events.filter((e) => (!category || e.category === category) && (!city || e.city === city)),
    [events, category, city],
  );

  return (
    <SafeAreaView style={st.safe} edges={['top']}>
      <FlatList
        data={shown}
        keyExtractor={(e) => e.id}
        contentContainerStyle={st.list}
        refreshControl={<RefreshControl refreshing={loading} onRefresh={load} tintColor={color.textMuted} />}
        ListHeaderComponent={
          <View>
            <Title>Events</Title>
            <Muted style={st.blurb}>Pick a show, then pick your seat.</Muted>

            {failed ? <Banner kind="error" title="No connection" text={failed} /> : null}

            {kinds.length > 1 ? (
              <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={st.pills}>
                {([['', 'All'], ...kinds.map((k) => [k, CATEGORY_NAMES[k]] as const)] as const).map(([value, text]) => (
                  <Pressable
                    key={value || 'all'}
                    onPress={() => setCategory(value as Category | '')}
                    style={[st.pill, category === value && st.pillOn]}>
                    <Text style={[st.pillText, category === value && st.pillTextOn]}>{text}</Text>
                  </Pressable>
                ))}
              </ScrollView>
            ) : null}

            {cities.length > 1 ? (
              <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={st.pills}>
                {[['', 'All cities'], ...cities.map((c) => [c, c])].map(([value, text]) => (
                  <Pressable
                    key={value || 'all-cities'}
                    onPress={() => setCity(value)}
                    style={[st.pill, st.cityPill, city === value && st.pillOn]}>
                    <Ionicons
                      name="location"
                      size={14}
                      color={city === value ? color.onPrimary : color.textMuted}
                    />
                    <Text style={[st.pillText, city === value && st.pillTextOn]}>{text}</Text>
                  </Pressable>
                ))}
              </ScrollView>
            ) : null}
          </View>
        }
        ListEmptyComponent={
          loading ? null : events.length === 0 ? (
            <Empty
              icon="calendar-outline"
              title="Nothing on sale yet."
              text="New events show up here the moment they open. Check back soon."
            />
          ) : (
            <Empty
              icon="search-outline"
              title="Nothing matches that."
              text="Try another city or another kind of night out."
              action={{
                label: 'Clear the filters',
                onPress: () => {
                  setCategory('');
                  setCity('');
                },
              }}
            />
          )
        }
        renderItem={({ item }) => <EventCard event={item} />}
      />
    </SafeAreaView>
  );
}

function EventCard({ event }: { event: Event }) {
  const when = new Date(event.startsAt);
  const mon = when.toLocaleString(undefined, { month: 'short' }).toUpperCase();
  const dow = when.toLocaleString(undefined, { weekday: 'short' }).toUpperCase();
  const time = when.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });

  return (
    <Pressable
      disabled={event.cancelled}
      onPress={() => router.push(`/event/${event.id}`)}
      style={({ pressed }) => [st.card, event.cancelled && st.cardOff, pressed && { opacity: 0.85 }]}>
      <Poster id={event.id} name={event.name} imageUrl={event.imageUrl} height={190}>
        <View style={st.dateChip}>
          <Text style={st.chipMon}>{mon}</Text>
          <Text style={st.chipDay}>{when.getDate()}</Text>
          <Text style={st.chipDow}>{dow}</Text>
        </View>

        {event.cancelled || event.hasStarted ? (
          <View style={st.cornerTag}>
            <Tag text={event.cancelled ? 'Cancelled' : 'Started'} tone={event.cancelled ? 'danger' : 'onPoster'} />
          </View>
        ) : null}
      </Poster>

      <View style={st.cardBody}>
        <View style={st.cardWords}>
          <Text style={st.kind}>{CATEGORY_NAMES[event.category]}</Text>
          <Text style={st.name}>{event.name}</Text>
          <Muted style={st.where}>{[event.venue, event.city].filter(Boolean).join(' · ')}</Muted>
          <Caption>{time}</Caption>
        </View>

        {!event.cancelled ? <Ionicons name="chevron-forward" size={20} color={color.textMuted} /> : null}
      </View>
    </Pressable>
  );
}

const st = StyleSheet.create({
  safe: { flex: 1, backgroundColor: color.bg },
  list: { padding: space.gutter, paddingBottom: 32, gap: 16 },
  blurb: { marginTop: 4, marginBottom: 18 },

  pills: { gap: 8, paddingBottom: 14, paddingRight: space.gutter },
  pill: {
    paddingHorizontal: 16,
    paddingVertical: 10,
    borderRadius: radius.pill,
    borderWidth: 1,
    borderColor: color.lineStrong,
    backgroundColor: color.surface,
  },
  cityPill: { flexDirection: 'row', alignItems: 'center', gap: 6 },
  pillOn: { backgroundColor: color.primary, borderColor: color.primary },
  pillText: { fontSize: 14, fontWeight: '600', color: color.textMuted },
  pillTextOn: { color: color.onPrimary },

  card: {
    backgroundColor: color.surface,
    borderWidth: 1,
    borderColor: color.line,
    borderRadius: radius.lg,
    overflow: 'hidden',
  },
  cardOff: { opacity: 0.55 },
  cardBody: { flexDirection: 'row', alignItems: 'center', gap: 12, padding: 16 },
  cardWords: { flex: 1 },

  dateChip: {
    position: 'absolute',
    top: 10,
    left: 10,
    paddingHorizontal: 11,
    paddingVertical: 7,
    borderRadius: 12,
    backgroundColor: color.surface,
    alignItems: 'center',
  },
  chipMon: { fontSize: 10, fontWeight: '600', letterSpacing: 1.2, color: color.primary },
  chipDay: { fontSize: 21, fontWeight: '700', color: color.text },
  chipDow: { fontSize: 10, color: color.textMuted },

  cornerTag: { position: 'absolute', top: 12, right: 12 },

  kind: { fontSize: 11, fontWeight: '700', letterSpacing: 1.1, color: color.primary, marginBottom: 5 },
  name: { fontSize: 19, lineHeight: 24, fontWeight: '700', color: color.text },
  where: { fontSize: 14 },
});
