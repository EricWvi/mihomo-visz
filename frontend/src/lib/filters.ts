import { create } from 'zustand'
export type Direction = 'total' | 'upload' | 'download'
type Filters = {
  date: string; node: string; search: string; group: 'domain' | 'host'; direction: Direction; hour: number | null
  setDate: (date: string) => void; setNode: (node: string) => void; setSearch: (search: string) => void
  setGroup: (group: 'domain' | 'host') => void; setDirection: (direction: Direction) => void; setHour: (hour: number) => void
}
export const useFilters = create<Filters>((set) => ({
  date: '', node: 'all', search: '', group: 'domain', direction: 'total', hour: null,
  setDate: (date) => set({ date, hour: null, node: 'all' }), setNode: (node) => set({ node }),
  setSearch: (search) => set({ search }), setGroup: (group) => set({ group }),
  setDirection: (direction) => set({ direction }), setHour: (hour) => set({ hour }),
}))
