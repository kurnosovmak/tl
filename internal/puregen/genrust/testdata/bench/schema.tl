vector#1cb5c415 {t:Type} # [t] = Vector t;
bench.ints values:(vector int) = bench.Ints;
bench.bytes values:(vector string) = bench.Bytes;

bench.row fields_mask:# id:int
    label:fields_mask.0?string
    score:fields_mask.2?double
    samples:fields_mask.7?(vector int)
    timestamp:fields_mask.15?long
    detail:fields_mask.31?string
    extra_mask:# sequence:extra_mask.1?int tags:extra_mask.30?(vector string)
    = bench.Row;

bench.tick value:int = bench.Event;
bench.update row:bench.row = bench.Event;
bench.group name:string rows:(vector bench.row) = bench.Event;

bench.batch epoch:int events:(vector bench.Event) = bench.Batch;
