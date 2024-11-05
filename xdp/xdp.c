//go:build ignore

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>

// Ethernet header
struct ethhdr {
  __u8 h_dest[6];
  __u8 h_source[6];
  __u16 h_proto;
} __attribute__((packed));

// IPv4 header
struct iphdr {
  __u8 ihl : 4;
  __u8 version : 4;
  __u8 tos;
  __u16 tot_len;
  __u16 id;
  __u16 frag_off;
  __u8 ttl;
  __u8 protocol;
  __u16 check;
  __u32 saddr;
  __u32 daddr;
} __attribute__((packed));

// TCP header
struct tcphdr {
  __u16 source;
  __u16 dest;
  __u32 seq;
  __u32 ack_seq;
  union {
    struct {
      __u16 ns : 1,
            reserved : 3,
            doff : 4,
            fin : 1,
            syn : 1,
            rst : 1,
            psh : 1,
            ack : 1,
            urg : 1,
            ece : 1,
            cwr : 1;
    };
  };
  __u16 window;
  __u16 check;
  __u16 urg_ptr;
};

// 定義されたBPFマップ
struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, 128);
  __type(key, __u32);
  __type(value, __u16);
  __uint(map_flags, 1U << 0);
} targets SEC(".maps");

struct {
  __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
  __uint(max_entries, 1024);
} perfmap SEC(".maps");

// PerfEvent item
struct perf_event_item {
  __u32 src_ip, dst_ip;
  __u16 src_port, dst_port;
  __u32 seq;
};
_Static_assert(sizeof(struct perf_event_item) == 16, "wrong size of perf_event_item");


__u16 ntohs(__u16 netshort) {
    return (netshort << 8) | (netshort >> 8);
}

__u32 ntohl(__u32 netlong) {
    return ((netlong & 0x000000FF) << 24) |
           ((netlong & 0x0000FF00) << 8)  |
           ((netlong & 0x00FF0000) >> 8)  |
           ((netlong & 0xFF000000) >> 24);
}

SEC("xdp")
int wrangell(struct xdp_md *ctx) {
  void *data_end = (void *)(long)ctx->data_end;
  void *data = (void *)(long)ctx->data;
  __u64 packet_size = data_end - data;

  // L2
  struct ethhdr *ether = data;
  if (data + sizeof(*ether) > data_end) {
    return XDP_ABORTED;
  }

  // L3
  if (ether->h_proto != 0x08) {  // htons(ETH_P_IP) -> 0x08
                                 // Non IPv4
    return XDP_PASS;
  }
  data += sizeof(*ether);
  struct iphdr *ip = data;
  if (data + sizeof(*ip) > data_end) {
    return XDP_ABORTED;
  }

  // L4
  if (ip->protocol != 0x06) {  // IPPROTO_TCP -> 6
    return XDP_PASS;
  }
  data += ip->ihl * 4;
  struct tcphdr *tcp = data;
  if (data + sizeof(*tcp) > data_end) {
    return XDP_ABORTED;
  }

  if (tcp->syn && !tcp->ack) {
    __u64 key = ntohl(ip->daddr);
    __u16 *value = bpf_map_lookup_elem(&targets, &key);
    if (value && *value == ntohs(tcp->dest)) {
      struct perf_event_item evt = {
        .src_ip = ip->saddr,
        .dst_ip = ip->daddr,
        .src_port = tcp->source,
        .dst_port = tcp->dest,
        .seq = tcp->seq,
      };
      __u64 flags = BPF_F_CURRENT_CPU | (packet_size << 32);
      bpf_perf_event_output(ctx, &perfmap, flags, &evt, sizeof(evt));

      return XDP_DROP;
    }
  }

  return XDP_PASS;
}


char __license[] SEC("license") = "Dual MIT/GPL";
