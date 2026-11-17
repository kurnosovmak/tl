@read sample.get#12345678 id:int => Int;
@any rpcDestActor#87654321 {X:Type} actor:long query:!X => X;
@any rpcProxy.diagonal#22222222 {X:Type} query:!X => Vector (Maybe X);
