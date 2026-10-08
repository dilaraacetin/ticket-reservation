// One event. Everything somebody wants before they commit to a seat, and one
// button to go and take one.

import { Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import { router, useLocalSearchParams } from 'expo-router';
import { useEffect, useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView, useSafeAreaInsets } from 'react-native-safe-area-context';

import { Banner, Button, Caption, Heading, Label, Muted, Poster, Tag } from '@/components/ui';
import { api } from '@/lib/api';
import { color, radius, space } from '@/lib/theme';
import { CATEGORY_NAMES, type EventDetail } from '@/lib/types';

export default function EventScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const insets = useSafeAreaInsets();

  const [event, setEvent] = useState<EventDetail | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    (async () => {
      try {
        setEvent(await api<EventDetail>('GET', `/events/${encodeURIComponent(id)}`));
      } catch {
        setFailed(true);
      }
    })();
  }, [id]);

  if (failed) {
    return (
      <SafeAreaView style={st.safe}>
        <View style={st.pad}>
          <Banner kind="error" title="Could not load this event" text="It may have been withdrawn." />
          <Button title="Back to events" variant="secondary" onPress={() => router.back()} />
        </View>
      </SafeAreaView>
    );
  }

  if (!event) {
    return (
      <View style={[st.safe, st.centre]}>
        <ActivityIndicator color={color.primary} />
      </View>
    );
  }

  const when = new Date(event.startsAt);
  const sold = event.total - event.available;

  return (
    <View style={st.safe}>
      <ScrollView contentContainerStyle={st.scroll} bounces={false}>
        <Poster id={event.id} name={event.name} imageUrl={event.imageUrl} height={300}>
          <LinearGradient
            colors={['transparent', 'rgba(0,0,0,0.78)']}
            style={StyleSheet.absoluteFill}
            locations={[0.35, 1]}
          />
          <View style={st.over}>
            <View style={st.tags}>
              <Tag text={CATEGORY_NAMES[event.category]} tone="onPoster" />
              {event.cancelled ? <Tag text="Cancelled" tone="danger" /> : null}
              {!event.cancelled && event.hasStarted ? <Tag text="Started" tone="onPoster" /> : null}
            </View>
            <Text style={st.heroTitle}>{event.name}</Text>
          </View>
        </Poster>

        <Pressable
          onPress={() => router.back()}
          hitSlop={10}
          style={[st.back, { top: insets.top + 8 }]}
          accessibilityLabel="Back to events">
          <Ionicons name="chevron-back" size={22} color={color.text} />
        </Pressable>

        <View style={st.pad}>
          <Fact icon="calendar-outline" label="When" value={when.toLocaleString(undefined, {
            weekday: 'long', day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit',
          })} />
          <Fact icon="location-outline" label="Where" value={[event.venue, event.city].filter(Boolean).join(', ')} />

          {event.total > 0 ? (
            <View style={[st.avail, event.available === 0 && st.availNone]}>
              <View style={st.availRow}>
                <Text style={[st.availN, event.available === 0 && { color: color.danger }]}>
                  {event.available}
                </Text>
                <Muted>of {event.total} seats open</Muted>
              </View>
              <View style={st.meter}>
                <View style={[st.meterFill, { width: `${Math.round((sold / event.total) * 100)}%` }]} />
              </View>
              {event.available === 0 ? (
                <Caption style={st.availNote}>
                  Every seat is taken. Join the line and you get the next one that frees up.
                </Caption>
              ) : null}
            </View>
          ) : null}

          {event.description ? <Prose title="About this event" text={event.description} /> : null}
          {event.rules ? <Prose title="Good to know" text={event.rules} /> : null}

          <View style={st.action}>
            {event.cancelled ? (
              <Muted style={st.centred}>
                This event has been cancelled. Tickets already bought stay valid as a record.
              </Muted>
            ) : (
              <>
                <Button
                  title={event.available === 0 ? 'Join the waiting list' : 'Pick your seat'}
                  onPress={() => router.push(`/event/${event.id}/seats`)}
                />
                {event.hasStarted ? (
                  <Caption style={st.centred}>
                    This event has already started. Seats may still be open.
                  </Caption>
                ) : null}
              </>
            )}
          </View>
        </View>
      </ScrollView>
    </View>
  );
}

function Fact({ icon, label, value }: { icon: keyof typeof Ionicons.glyphMap; label: string; value: string }) {
  return (
    <View style={st.fact}>
      <View style={st.factMark}>
        <Ionicons name={icon} size={20} color={color.primary} />
      </View>
      <View style={st.fill}>
        <Label>{label.toUpperCase()}</Label>
        <Text style={st.factValue}>{value}</Text>
      </View>
    </View>
  );
}

// Paragraphs rather than one block: text written with blank lines in it should
// read the way it was written.
function Prose({ title, text }: { title: string; text: string }) {
  return (
    <View style={st.prose}>
      <Heading style={st.proseTitle}>{title}</Heading>
      {text
        .split(/\n{2,}/)
        .filter((p) => p.trim())
        .map((p, i) => (
          <Muted key={i} style={st.proseBody}>
            {p.trim()}
          </Muted>
        ))}
    </View>
  );
}

const st = StyleSheet.create({
  safe: { flex: 1, backgroundColor: color.bg },
  centre: { alignItems: 'center', justifyContent: 'center' },
  scroll: { paddingBottom: 40 },
  pad: { padding: space.gutter },
  fill: { flex: 1 },
  centred: { textAlign: 'center' },

  back: {
    position: 'absolute',
    left: 12,
    width: 40,
    height: 40,
    borderRadius: 20,
    backgroundColor: color.surface,
    alignItems: 'center',
    justifyContent: 'center',
  },

  over: { position: 'absolute', left: 0, right: 0, bottom: 0, padding: space.gutter, gap: 8 },
  tags: { flexDirection: 'row', gap: 8, flexWrap: 'wrap' },
  heroTitle: { fontSize: 30, lineHeight: 34, fontWeight: '700', color: '#FFFFFF' },

  fact: { flexDirection: 'row', alignItems: 'center', gap: 14, paddingVertical: 12 },
  factMark: {
    width: 40,
    height: 40,
    borderRadius: 12,
    backgroundColor: color.surfaceRaised,
    alignItems: 'center',
    justifyContent: 'center',
  },
  factValue: { fontSize: 16, lineHeight: 21, fontWeight: '600', color: color.text },

  avail: {
    backgroundColor: color.surface,
    borderWidth: 1,
    borderColor: color.line,
    borderRadius: radius.base,
    padding: 16,
    marginTop: 10,
  },
  availNone: { borderColor: color.danger },
  availRow: { flexDirection: 'row', alignItems: 'baseline', gap: 8 },
  availN: { fontSize: 30, fontWeight: '700', color: color.success },
  meter: { height: 5, borderRadius: 3, backgroundColor: color.lineStrong, marginTop: 12, overflow: 'hidden' },
  meterFill: { height: '100%', backgroundColor: color.primary },
  availNote: { marginTop: 10 },

  prose: { marginTop: 26 },
  proseTitle: { fontSize: 20, marginBottom: 8 },
  proseBody: { lineHeight: 24, marginBottom: 10 },

  action: { marginTop: 28, gap: 10 },
});
