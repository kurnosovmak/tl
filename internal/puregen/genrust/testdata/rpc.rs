// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

use gentl::JsonMode;
use gentl::basictl::Error;
use gentl::rpc::{Format, Request, Response};
use gentl::types::{
    CasesTL2TestFunctionNoDep1::CasesTL2TestFunctionNoDep1, RpcDestActor::RpcDestActor,
    RpcProxyDiagonal::RpcProxyDiagonal, SampleGet::SampleGet,
};

#[test]
fn go_result() {
    let request = Request::CasesTL2TestFunctionNoDep1(CasesTL2TestFunctionNoDep1 { X: 42 });
    for (format, expected) in [
        (Format::Tl1, include_bytes!("rpc_result.bin").as_slice()),
        (Format::Tl2, include_bytes!("rpc_result.tl2").as_slice()),
    ] {
        let mut input = expected;
        let response = request.read_response(&mut input, format).unwrap();
        let Response::CasesTL2TestFunctionNoDep1(value) = &response else {
            panic!("wrong response type");
        };
        assert_eq!(**value, -17);
        assert!(input.is_empty());
        let mut output = Vec::new();
        request
            .write_response(&response, &mut output, format)
            .unwrap();
        assert_eq!(output, expected);

        let mut request_bytes = Vec::new();
        request.write(&mut request_bytes, format).unwrap();
        let mut request_input = request_bytes.as_slice();
        let Request::CasesTL2TestFunctionNoDep1(decoded) =
            Request::read(&mut request_input, format).unwrap()
        else {
            panic!("wrong request type");
        };
        assert_eq!(decoded.X, 42);
        assert!(request_input.is_empty());
    }
}

#[test]
fn generic_requests() {
    let request = Request::RpcDestActor(RpcDestActor {
        Actor: 7,
        Query: Some(Box::new(Request::SampleGet(SampleGet { Id: 42 }))),
    });
    let mut encoded = Vec::new();
    request.write_tl1(&mut encoded).unwrap();
    assert_eq!(
        encoded,
        [
            0x21, 0x43, 0x65, 0x87, 7, 0, 0, 0, 0, 0, 0, 0, 0x78, 0x56, 0x34, 0x12, 42, 0, 0, 0,
        ]
    );
    let mut bytes = encoded.as_slice();
    let decoded = Request::read_tl1(&mut bytes).unwrap();
    assert!(bytes.is_empty());
    let Request::RpcDestActor(actor) = decoded else {
        panic!("wrong wrapper");
    };
    assert_eq!(actor.Actor, 7);
    let Some(Request::SampleGet(get)) = actor.Query.as_deref() else {
        panic!("wrong inner request");
    };
    assert_eq!(get.Id, 42);

    let mut encoded = Vec::new();
    request.write_json(&mut encoded, JsonMode::Legacy).unwrap();
    let decoded = Request::read_json(&encoded, JsonMode::Legacy).unwrap();
    let mut output = Vec::new();
    decoded.write_json(&mut output, JsonMode::Legacy).unwrap();
    assert_eq!(output, encoded);
}

#[test]
fn generic_nested_results() {
    let request = RpcProxyDiagonal {
        Query: Some(Box::new(Request::SampleGet(SampleGet { Id: 1 }))),
    };
    let value = vec![Some(Some(Box::new(Response::SampleGet(Box::new(9))))), None];
    let mut encoded = Vec::new();
    request.write_result_tl1(&value, &mut encoded).unwrap();
    assert_eq!(
        encoded,
        [
            0x15, 0xc4, 0xb5, 0x1c, 2, 0, 0, 0, 0xf8, 0x8e, 0x9c, 0x3f, 0xda, 0x9b, 0x50, 0xa8, 9,
            0, 0, 0, 0x7b, 0x0a, 0x93, 0x27,
        ]
    );
    let mut input = encoded.as_slice();
    let decoded = request.read_result_tl1(&mut input).unwrap();
    assert!(input.is_empty());
    let Some(Response::SampleGet(result)) = decoded[0].as_ref().and_then(Option::as_deref) else {
        panic!("unexpected response");
    };
    assert_eq!(**result, 9);
    assert!(decoded[1].is_none());

    let bad = vec![Some(Some(Box::new(Response::RpcDestActor(Box::new(None)))))];
    let before = encoded.clone();
    assert_eq!(
        request.write_result_tl1(&bad, &mut encoded),
        Err(Error::WrongRpcResponse)
    );
    assert_eq!(encoded, before);
}
